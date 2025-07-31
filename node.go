package webfs

// NodeInfo provides read-only access to node information for external consumers
type NodeInfo interface {
	// Name returns the node's name (last path component)
	Name() string

	// NodeID returns the unique node identifier
	NodeID() uint64

	// Path returns the full path to the node
	Path() string

	// IsDel returns true if the node is marked for deletion
	IsDel() bool
}

// FileSystemOperator defines the core filesystem operations that external consumers need
type FileSystemOperator interface {
	Root() NodeInfo
	AddFileNode(req *FileCreateRequest) (NodeInfo, error)
	AddDirNode(req *DirCreateRequest) (NodeInfo, error)
}
