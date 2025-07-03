package core

import "github.com/hanwen/go-fuse/v2/fuse"

// NodeContext wraps a locked [Node] (plus any upstream locks).
// Calling NodeContext.Close() unwinds all unlocking/cleanup callbacks in reverse order.
// Do NOT invoke any locking methods on the raw Node or its embedded Inode while
// this context is active. Use only the safe snapshot and update helpers below.
//
// NOTE: NodeContext itself is **not** thread-safe meaning references
// to it should not be shared between goroutines
type NodeContext struct {
	node             *Node
	closeFns         []func()
	inodeReadLocked  bool // tracks if Inode read-lock is held
	inodeWriteLocked bool // tracks if Inode write-lock is held
}

// NewNodeContext RLocks the Node returns a new NodeContext for a given Node
func NewNodeContext(node *Node) *NodeContext {
	node.mu.RLock()
	ctx := &NodeContext{node: node}
	ctx.AddClose(node.mu.RUnlock)
	return ctx
}

func (ctx *NodeContext) NodeID() uint64 {
	return ctx.node.NodeID()
}

// Attr returns a snapshot of the fuse attributes.
func (ctx *NodeContext) Attr() fuse.Attr {
	// brief inode read-lock & release
	return ctx.node.CopyAttr()
}

// UpdateAttr runs fn under the Inode write-lock for atomic modifications.
func (ctx *NodeContext) UpdateAttr(fn func(attr *fuse.Attr)) {
	ctx.node.Inode.mu.Lock()
	defer ctx.node.Inode.mu.Unlock()
	fn(ctx.node.fuseAttr)
}

// Name returns the node's immutable Name.
func (ctx *NodeContext) Name() string {
	return ctx.node.name
}

// Children returns child names under the existing node RLock.
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

// AddClose pushes a cleanup callback (e.g., unlock) onto the end of the stack.
func (ctx *NodeContext) AddClose(fn func()) {
	ctx.closeFns = append(ctx.closeFns, fn)
}

// Close unwinds all cleanup callbacks in reverse order.
// Safe to call even if ctx is nil or no locks were acquired; it is
// a no-op in those cases, so you can `defer ctx.Close()` unconditionally.
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
