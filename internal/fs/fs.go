package fs

import (
	"os"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/brettbedarf/httpfs/internal/nodes"
	"github.com/brettbedarf/httpfs/util"
	"github.com/brettbedarf/webfs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

type WebFs struct {
	root         *nodes.Node             // Root of node tree
	inodeMap     map[uint64]*nodes.Inode // Map of inode numbers to Inodes
	nextIno      atomic.Uint64           // Next inode number to assign
	inodeMapLock sync.RWMutex
}

func NewWebFs(mountPoint string, config *webfs.Config) *WebFs {
	now := time.Now()
	nowUnix := uint64(now.Unix())
	nowNano := uint32(now.Nanosecond())

	rootAttr := fuse.Attr{
		Ino:       fuse.FUSE_ROOT_ID,
		Size:      0,
		Blocks:    0,
		Atime:     nowUnix,
		Mtime:     nowUnix,
		Ctime:     nowUnix,
		Atimensec: nowNano,
		Mtimensec: nowNano,
		Ctimensec: nowNano,
		Mode:      uint32(syscall.S_IFDIR | 0755), // directory with rwxr-xr-x permissions
		Nlink:     1,
		Owner: fuse.Owner{
			Uid: uint32(os.Getuid()),
			Gid: uint32(os.Getgid()),
		},
		Blksize: 4096, // preferred size for fs ops
		// Only non-zero for device files (see S_IFCHR and S_IFBLK) (N/A)
		Rdev: 0,
		// Used for byte-to-byte compat with fuse wire protocol
		// but *should be handled by go-fuse or not relevant and just
		// set/default to 0
		Padding: 0,
	}

	rootInode := nodes.NewInode(rootAttr)
	rootNode := nodes.NewNode("", rootInode)

	inodeMap := make(map[uint64]*nodes.Inode)
	inodeMap[rootAttr.Ino] = rootInode

	tree := WebFs{
		root:     rootNode,
		inodeMap: inodeMap,
	}
	tree.nextIno.Store(fuse.FUSE_ROOT_ID + 1)

	return &tree
}

func (fs *WebFs) AddNodeFromDef(fileDef *nodes.FileSrcDef) *nodes.Node {
	logger := util.GetLogger("FileStore.AssignNode")
	logger.Debug().Interface("fileDef", fileDef).Msg("Adding FSNode")
	var inode *nodes.Inode
	var node *nodes.Node

	// TODO: overwrite/suffix '_1'/err?
	nodePath = strings.TrimSpace(nodePath)
	nodePath = path.Clean(nodePath)
	// dirPath paths should always end in "/" and the file split will be empty string
	dirPath, file := path.Split(nodePath)

	// trim any leading "/" so that we will always start at the first child of root
	// we are assuming that everything is relative to root so
	// "foo/bar" and "/foo/bar" are equivalent
	dirPath = strings.TrimPrefix(dirPath, "/")
	curNode := fs.root
	for _, dir := range strings.Split(dirPath, "/") {
		if child, exists := curNode.Children[dir]; exists {
			curNode = child
		} else {
			// Create new directory child node
		}
	}

	ino := fs.nextIno
	fs.nextIno++

	return *Node{}
}

// func (store *FileStore) GetFsNodeForPath(path string) *Node {
// 	return nil
// }

//
// func (store *FileStore) GetFsNodeForPath(path string) *FsNode {
// 	now := time.Now()
// 	node := store.pathMap[path]
// 	// TODO:return from cache or get web header
// 	return &fuse.Attr{
// 		Ino:       node,
// 		Size:      0,
// 		Blocks:    0,
// 		Atime:     uint64(now.Unix()),
// 		Mtime:     uint64(now.Unix()),
// 		Ctime:     uint64(now.Unix()),
// 		Atimensec: uint32(now.Nanosecond()),
// 		Mtimensec: uint32(now.Nanosecond()),
// 		Ctimensec: uint32(now.Nanosecond()),
// 		Mode:      uint32(syscall.S_IFREG | 0444), // regular file with r--r--r-- permissions
// 		Nlink:     1,
// 		Owner: fuse.Owner{
// 			Uid: uint32(os.Getuid()),
// 			Gid: uint32(os.Getgid()),
// 		},
// 		Rdev:    0,
// 		Blksize: 4096,
// 	}
// }
