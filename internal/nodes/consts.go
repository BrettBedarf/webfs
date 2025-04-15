package nodes

import "syscall"

type SysAttrType uint

const (
	DirAttr     SysAttrType = syscall.S_IFDIR
	FileAttr                = syscall.S_IFREG
	SymlinkAttr             = syscall.S_IFLNK
	SockAttr                = syscall.S_IFSOCK
)
