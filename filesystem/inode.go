package filesystem

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/brettbedarf/webfs"
	"github.com/brettbedarf/webfs/internal/util"
	"github.com/hanwen/go-fuse/v2/fuse"
)

var ErrNoAdapters = errors.New("adapter pool not set")

type Inode struct {
	// Low-level fuse wire protocol attributes; Only access directly if
	// handling locks manually
	attr        *fuse.Attr
	adapterPool *adapterPool
	hLinks      []*Node // Hard links to this inode
	sLinks      []*Node // Symbolic links to this inode

	// Data cache - stores actual file content blocks
	dataCache *dataCache

	mu  sync.RWMutex
	log util.Logger
}

// CachedRange represents a contiguous range of cached data
type CachedRange struct {
	start int64
	end   int64 // exclusive
	data  []byte
}

// dataCache manages cached byte ranges for an inode
type dataCache struct {
	ranges []CachedRange
	mu     sync.RWMutex
}

func NewInode(attr *fuse.Attr, adapters []webfs.FileAdapter) *Inode {
	return &Inode{
		attr:        attr,
		adapterPool: newAdapterPool(adapters),
		dataCache:   newDataCache(),
		hLinks:      make([]*Node, 0, 1), // 1 init capacity since most inodes expected to have only themselves
		sLinks:      make([]*Node, 0),
		log:         util.GetLogger("Inode").With().Uint64("ino", attr.Ino).Logger(),
	}
}

// addHardLinkLocked appends a new hard link to the inode.
// Caller must hold n.mu.Lock().
func (n *Inode) addHardLinkLocked(node *Node) {
	n.hLinks = append(n.hLinks, node)
	n.attr.Nlink++
}

// AddHardLink adds a new Node, including the initial, as hard link
func (n *Inode) AddHardLink(node *Node) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.addHardLinkLocked(node)
}

// CopyAttr returns a copy of the inode's attributes
func (n *Inode) CopyAttr() fuse.Attr {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return *n.attr
}

// RefreshMeta refreshes the inode's metadata in a background goroutine
func (n *Inode) RefreshMeta() {
	n.log.Debug().Msg("Refreshing metadata")
	if n.adapterPool == nil {
		n.log.Warn().Msg("Adapter pool not set")
		return
	}
	go func() {
		opsCtx := context.Background()
		err := n.adapterPool.tryOperation(opsCtx, func(ctx context.Context, adapter *inodeAdapter) error {
			meta, err := adapter.GetMeta(ctx)
			if err != nil {
				return err
			}
			n.log.Trace().Interface("meta", meta).Msg("Retrieved metadata")
			n.mu.Lock()
			defer n.mu.Unlock()
			n.attr.Size = meta.Size
			n.attr.SetTimes(nil, meta.LastModified, nil)
			// TODO: figure out how to handle Version and TTL
			return nil
		})
		if err != nil {
			n.log.Error().Err(err).Msg("Failed to refresh metadata after trying all adapters")
		}
	}()
}

// Read reads data from the inode, using cache when available or fetching from adapters
func (n *Inode) Read(ctx context.Context, offset int64, size int64) ([]byte, error) {
	if n.adapterPool == nil {
		return nil, ErrNoAdapters
	}

	// Check cache first
	if cached := n.dataCache.get(offset, size); cached != nil {
		n.log.Trace().Int64("offset", offset).Int64("size", size).Msg("Cache hit")
		return cached, nil
	}

	// Cache miss - fetch from adapter
	n.log.Trace().Int64("offset", offset).Int64("size", size).Msg("Cache miss - fetching from adapter")

	// Allocate buffer for adapter to read into
	buf := make([]byte, size)
	var bytesRead int

	err := n.adapterPool.tryOperation(ctx, func(ctx context.Context, adapter *inodeAdapter) error {
		var err error
		bytesRead, err = adapter.Read(ctx, offset, size, buf)
		return err
	})
	if err != nil {
		n.log.Error().Err(err).Msg("Failed to read with all adapters")
		return nil, err
	}

	// Trim buffer to actual bytes read
	data := buf[:bytesRead]

	// Cache the fetched data
	n.dataCache.store(offset, data)

	return data, nil
}

// IsCached returns true if the specified range is fully cached
func (n *Inode) IsCached(offset int64, size int64) bool {
	return n.dataCache.isCached(offset, size)
}

// GetCachedRanges returns all currently cached ranges (for debugging/monitoring)
func (n *Inode) GetCachedRanges() []CachedRange {
	return n.dataCache.getRanges()
}

// ClearCache clears all cached data
func (n *Inode) ClearCache() {
	n.dataCache.clear()
}

type inodeAdapter struct {
	webfs.FileAdapter
	isHealthy   atomic.Bool
	lastAttempt time.Time
	failCount   int32
	maxRetries  int32
}

type adapterPool struct {
	primary    *inodeAdapter
	backups    []*inodeAdapter
	mu         sync.RWMutex
	log        util.Logger
	retryDelay time.Duration
}

const (
	defaultMaxRetries = 3
	defaultRetryDelay = 30 * time.Second
)

func newAdapterPool(adapters []webfs.FileAdapter) *adapterPool {
	if len(adapters) == 0 {
		return nil
	}

	pool := &adapterPool{
		log:        util.GetLogger("AdapterPool"),
		retryDelay: defaultRetryDelay,
	}

	// First adapter is primary
	pool.primary = &inodeAdapter{
		FileAdapter: adapters[0],
		maxRetries:  defaultMaxRetries,
	}
	pool.primary.isHealthy.Store(true)

	// Rest are backups
	for i := 1; i < len(adapters); i++ {
		backup := &inodeAdapter{
			FileAdapter: adapters[i],
			maxRetries:  defaultMaxRetries,
		}
		backup.isHealthy.Store(true)
		pool.backups = append(pool.backups, backup)
	}

	return pool
}

// tryOperation executes an operation with automatic failover to backup adapters
func (p *adapterPool) tryOperation(ctx context.Context, operation func(context.Context, *inodeAdapter) error) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Build list of adapters to try
	adapters := make([]*inodeAdapter, 0, len(p.backups)+1)

	// Add primary if healthy or should retry
	if p.primary.isHealthy.Load() || p.primary.shouldRetry() {
		adapters = append(adapters, p.primary)
	}

	// Add healthy backups
	for _, backup := range p.backups {
		if backup.isHealthy.Load() || backup.shouldRetry() {
			adapters = append(adapters, backup)
		}
	}

	// If no healthy adapters, try primary anyway as last resort
	if len(adapters) == 0 {
		adapters = append(adapters, p.primary)
	}

	var lastErr error
	for _, adapter := range adapters {
		// Check if context was cancelled before trying next adapter
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := operation(ctx, adapter); err != nil {
			p.log.Debug().Err(err).Msg("Adapter operation failed, trying next")
			adapter.markFailed()
			lastErr = err
			continue
		}

		// Operation succeeded, mark adapter as healthy
		adapter.markHealthy()
		return nil
	}

	// All adapters failed
	return lastErr
}

func (a *inodeAdapter) markFailed() {
	a.isHealthy.Store(false)
	atomic.AddInt32(&a.failCount, 1)
	a.lastAttempt = time.Now()
}

func (a *inodeAdapter) markHealthy() {
	a.isHealthy.Store(true)
	atomic.StoreInt32(&a.failCount, 0)
}

func (a *inodeAdapter) shouldRetry() bool {
	if a.isHealthy.Load() {
		return true
	}

	failCount := atomic.LoadInt32(&a.failCount)
	if failCount >= a.maxRetries {
		return false
	}

	return time.Since(a.lastAttempt) > time.Duration(failCount)*time.Second
}

// newDataCache creates a new empty data cache
func newDataCache() *dataCache {
	return &dataCache{
		ranges: make([]CachedRange, 0),
	}
}

// get returns cached data for the specified range, or nil if not fully cached
func (dc *dataCache) get(offset int64, size int64) []byte {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	end := offset + size

	// Look for a range that contains the entire requested range
	for _, r := range dc.ranges {
		if r.start <= offset && r.end >= end {
			// Found containing range, extract the requested slice
			startIdx := offset - r.start
			endIdx := startIdx + size
			return r.data[startIdx:endIdx]
		}
	}

	return nil
}

// store caches data starting at the given offset
func (dc *dataCache) store(offset int64, data []byte) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	if len(data) == 0 {
		return
	}

	newRange := CachedRange{
		start: offset,
		end:   offset + int64(len(data)),
		data:  make([]byte, len(data)),
	}
	copy(newRange.data, data)

	// TODO: merge overlapping ranges, handle fragmentation
	// For now, just append (basic implementation)
	dc.ranges = append(dc.ranges, newRange)
}

// isCached returns true if the specified range is fully cached
func (dc *dataCache) isCached(offset int64, size int64) bool {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	end := offset + size

	for _, r := range dc.ranges {
		if r.start <= offset && r.end >= end {
			return true
		}
	}

	return false
}

// getRanges returns a copy of all cached ranges
func (dc *dataCache) getRanges() []CachedRange {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	ranges := make([]CachedRange, len(dc.ranges))
	copy(ranges, dc.ranges)
	return ranges
}

// clear removes all cached data
func (dc *dataCache) clear() {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	dc.ranges = dc.ranges[:0]
}
