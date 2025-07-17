package filesystem

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/brettbedarf/webfs"
	"github.com/brettbedarf/webfs/internal/util"
	"github.com/hanwen/go-fuse/v2/fuse"
)

type Inode struct {
	// Low-level fuse wire protocol attributes; Only access directly if
	// handling locks manually
	attr        *fuse.Attr
	adapterPool *adapterPool
	hLinks      []*Node // Hard links to this inode
	sLinks      []*Node // Symbolic links to this inode
	mu          sync.RWMutex
	log         util.Logger
}

func NewInode(attr *fuse.Attr, adapters []webfs.FileAdapter) *Inode {
	return &Inode{
		attr:        attr,
		adapterPool: newAdapterPool(adapters),
		hLinks:      make([]*Node, 0, 1), // 1 init capacity since most inodes expected to have only themselves
		sLinks:      make([]*Node, 0),    // 0 init capacity since assumed low usage
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

// Read reads data from the inode using the adapter pool with automatic failover
func (n *Inode) Read(ctx context.Context, offset int64, size int64) ([]byte, error) {
	if n.adapterPool == nil {
		return nil, fmt.Errorf("adapter pool not set")
	}
	return nil, fmt.Errorf("Read not implemented")
	// var data []byte
	// err := n.adapterPool.tryOperation(ctx, func(ctx context.Context, adapter *inodeAdapter) error {
	// 	var err error
	// 	data, err = adapter.Read(ctx, offset, size)
	// 	return err
	// })
	// if err != nil {
	// 	n.log.Error().Err(err).Msg("Failed to read with all adapters")
	// 	return nil, err
	// }
	//
	// return data, nil
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
