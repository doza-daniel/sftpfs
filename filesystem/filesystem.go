package filesystem

import (
	"fmt"
	"log"
	"os"
	"sshfs/handle"
	"sshfs/inode"
	"sync"
	"time"

	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
	"github.com/pkg/sftp"
)

func New(sftpClient *sftp.Client) fuseutil.FileSystem {
	fs := &filesystem{}

	fs.inodes = make(map[fuseops.InodeID]inode.Inode)
	fs.nextInodeID = inodeIDGenerator(fuseops.RootInodeID + 10)

	fs.handles = make(map[fuseops.HandleID]handle.Handle)
	fs.nextHandleID = handleIDGenerator(0)

	fs.inodes[fuseops.RootInodeID] = fs.createRoot()
	fs.uid = 1000
	fs.gid = 1000
	fs.sftpClient = sftpClient
	fs.Mutex = &sync.Mutex{}

	fs.init()

	return fs
}

func inodeIDGenerator(first fuseops.InodeID) func() fuseops.InodeID {
	next := first
	return func() fuseops.InodeID {
		next++
		return next
	}
}

func handleIDGenerator(first fuseops.HandleID) func() fuseops.HandleID {
	next := first
	return func() fuseops.HandleID {
		next++
		return next
	}
}

type filesystem struct {
	inodes       map[fuseops.InodeID]inode.Inode
	nextHandleID func() fuseops.HandleID

	handles     map[fuseops.HandleID]handle.Handle
	nextInodeID func() fuseops.InodeID

	uid uint32
	gid uint32

	sftpClient *sftp.Client

	*sync.Mutex
}

// TODO error handling
func (fs *filesystem) init() {
	wd, err := fs.sftpClient.Getwd()
	if err != nil {
		panic(fmt.Errorf("failed to read working dir: %v", err))
	}

	entries, err := fs.sftpClient.ReadDir(wd)
	if err != nil {
		panic(err)
	}

	root := fs.inodes[fuseops.RootInodeID].(inode.DirInode)
	for _, entry := range entries {
		log.Printf("%+v", entry.Sys())

		in := fs.inodeFromRemoteDentry(entry)
		fs.inodes[in.InodeID()] = in
		root.AddEntry(entry.Name(), in)
	}
}

func (fs *filesystem) inodeFromRemoteDentry(entry os.FileInfo) inode.Inode {
	attrs := fuseops.InodeAttributes{
		Size:  uint64(entry.Size()),
		Nlink: 1,
		Mode:  entry.Mode(),
		Mtime: entry.ModTime(),
	}

	if entry.IsDir() {
		return inode.NewDir(fs.nextInodeID(), &attrs, entry.Name())
	}

	return inode.NewFile(fs.nextInodeID(), &attrs, entry.Name())
}

func (fs *filesystem) freshInode(parent inode.DirInode, name string, mode os.FileMode, isDir bool) inode.Inode {
	attrs := fs.freshAttributes(isDir, mode)

	var in inode.Inode
	if isDir {
		in = inode.NewDir(fs.nextInodeID(), &attrs, name)
	} else {
		in = inode.NewFile(fs.nextInodeID(), &attrs, name)
	}

	parent.AddEntry(name, in)

	return in
}

func (fs *filesystem) freshAttributes(isDir bool, mode os.FileMode) fuseops.InodeAttributes {
	var size uint64 = 0
	if isDir {
		size = 4096
		mode |= os.ModeDir
	}

	return fuseops.InodeAttributes{
		Size:  size,
		Nlink: 1,
		Uid:   fs.uid,
		Gid:   fs.gid,
		Mode:  mode,

		Atime:  time.Now(),
		Ctime:  time.Now(),
		Mtime:  time.Now(),
		Crtime: time.Now(),
	}
}

func (fs *filesystem) createRoot() inode.Inode {
	attrs := fuseops.InodeAttributes{
		Size:  4096,
		Nlink: 2,
		Uid:   fs.uid,
		Gid:   fs.gid,
		Mode:  0777 | os.ModeDir,

		Atime:  time.Now(),
		Ctime:  time.Now(),
		Mtime:  time.Now(),
		Crtime: time.Now(),
	}

	return inode.NewDir(fuseops.RootInodeID, &attrs, "/")
}
