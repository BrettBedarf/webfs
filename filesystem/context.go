package filesystem

import "github.com/hanwen/go-fuse/v2/fuse"

// NodeContext wraps a locked [Node] (plus any upstream locks).
// The Node is locked with parentMu.RLock() to protect access to parent and other fields.
// Children access within the context uses lock-free xsync.Map operations.
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

// NewNodeContext RLocks the Node and returns a new NodeContext for safe access
func NewNodeContext(node *Node) *NodeContext {
	node.mu.RLock()
	ctx := &NodeContext{node: node}
	ctx.AddClose(node.mu.RUnlock)
	return ctx
}

// Name returns the node's immutable Name.
func (ctx *NodeContext) Name() string {
	return ctx.node.name
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

// Children returns a slice of locked child node contexts
func (ctx *NodeContext) Children() []*NodeContext {
	children := make([]*NodeContext, 0, ctx.node.children.Size())
	ctx.node.children.Range(func(_ string, ch *Node) bool {
		children = append(children, NewNodeContext(ch))
		return true
	})
	return children
}

// UnsafeChildren returns pointers to the unlocked underlying child nodes in a new slice.
// Consumers are responsible to all lock/unlock mechanics.
func (ctx *NodeContext) UnsafeChildren() []*Node {
	children := make([]*Node, 0, ctx.node.children.Size())
	ctx.node.children.Range(func(_ string, ch *Node) bool {
		children = append(children, ch)
		return true
	})
	return children
}

// IterChildren iterates over child nodes with lock-free access within the locked context
// Each child node is read-locked before the callback is invoked and unlocked
// automatically after the callback returns.
func (ctx *NodeContext) IterChildren(fn func(ctx *NodeContext)) {
	ctx.node.children.Range(func(_ string, child *Node) bool {
		nc := NewNodeContext(child)
		fn(nc)
		nc.Close()
		return true
	})
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
