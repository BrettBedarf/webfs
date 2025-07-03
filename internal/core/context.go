package core

import "github.com/hanwen/go-fuse/v2/fuse"

// NodeContext wraps a locked [Node] (plus any upstream locks).
// Calling NodeContext.Close() unwinds all unlocking/cleanup callbacks in reverse order.
// Do NOT invoke any locking methods on the raw Node or its embedded Inode while
// this context is active. Use only the safe, snapshot and update helpers below.
//
// NOTE: NodeContext itself is **not** thread-safe meaning references
// to it should not be shared between goroutines
type NodeContext struct {
	node             *Node
	closeFns         []func()
	inodeWriteLocked bool // tracks if Inode write-lock is held
}

// AddClose pushes a cleanup callback (e.g., unlock) onto the end of the stack.
func (ctx *NodeContext) AddClose(fn func()) {
	ctx.closeFns = append(ctx.closeFns, fn)
}

// AddFrontClose pushes a cleanup callback onto the front of the stack.
func (ctx *NodeContext) AddFrontClose(fn func()) {
	ctx.closeFns = append([]func(){fn}, ctx.closeFns...)
}

// Close unwinds all cleanup callbacks in reverse order.
// Make sure to call this when you're done with the context!
//
// Example:
//
//	ctx := fs.GetChildCtx(parentID, name)
//	defer ctx.Close()
func (ctx *NodeContext) Close() {
	if ctx == nil {
		return
	}
	for i := len(ctx.closeFns) - 1; i >= 0; i-- {
		ctx.closeFns[i]()
	}
	ctx.closeFns = nil
}

// InodeRLock acquires the Inode's read-lock and schedules it to unlock first,
// but is a no-op if the Inode is already write-locked.
func (ctx *NodeContext) InodeRLock() {
	// if write-locked, readers are allowed without extra lock
	if ctx.inodeWriteLocked {
		return
	}
	ctx.node.Inode.mu.RLock()
	ctx.AddFrontClose(ctx.node.Inode.mu.RUnlock)
}

func (ctx *NodeContext) InodeLock() {
	if ctx.inodeWriteLocked {
		return
	}
	ctx.node.Inode.mu.Lock()
	ctx.inodeWriteLocked = true
	// schedule write-unlock at front: will run before any RUnlocks
	ctx.AddFrontClose(func() {
		ctx.node.Inode.mu.Unlock()
		ctx.inodeWriteLocked = false
	})
}

// Attr returns a snapshot of the fuse attributes.
// If the Inode is write-locked, it reads directly; otherwise it uses a read-lock.
func (ctx *NodeContext) Attr() fuse.Attr {
	if ctx.inodeWriteLocked {
		// safe direct copy under write lock
		return *ctx.node.Inode.fuseAttr
	}
	// brief read-lock for snapshot
	return ctx.node.Inode.CopyAttr()
}

// UpdateAttr runs fn under the Inode write-lock for atomic modifications.
func (ctx *NodeContext) UpdateAttr(fn func(attr *fuse.Attr)) {
	ctx.InodeLock()
	fn(ctx.node.Inode.fuseAttr)
}

// Name returns the node's immutable Name.
func (ctx *NodeContext) Name() string {
	return ctx.node.Name
}

// Children returns child names under the existing node.mu.RLock.
func (ctx *NodeContext) Children() []string {
	names := make([]string, 0, len(ctx.node.children))
	for name := range ctx.node.children {
		names = append(names, name)
	}
	return names
}

// HardLinkCount returns the number of hard links (Nlink).
func (ctx *NodeContext) HardLinkCount() uint64 {
	return uint64(ctx.Attr().Nlink)
}
