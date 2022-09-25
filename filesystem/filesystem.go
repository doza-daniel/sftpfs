package filesystem

import (
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

	fs.uid = 1000
	fs.gid = 1000
	fs.sftpClient = sftpClient
	fs.Mutex = &sync.Mutex{}

	fs.createRoot()

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

func (fs *filesystem) createRoot() {
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

	remotePath, err := fs.sftpClient.Getwd()
	if err != nil {
		panic(err) // TODO
	}

	rootDir := inode.NewDir(fuseops.RootInodeID, &attrs, remotePath, fs.sftpClient)

	fs.inodes[fuseops.RootInodeID] = rootDir
}
