package core

import "syscall"

type SysAttrType uint32

const (
	DirAttr     SysAttrType = syscall.S_IFDIR
	FileAttr                = syscall.S_IFREG
	SymlinkAttr             = syscall.S_IFLNK
	SockAttr                = syscall.S_IFSOCK
)
