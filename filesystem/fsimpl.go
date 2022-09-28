package filesystem

import (
	"context"
	"log"
	"os"
	"path"
	"sftpfs/handle"
	"sftpfs/inode"
	"time"

	"github.com/jacobsa/fuse"
	"github.com/jacobsa/fuse/fuseops"
)

// fuseutil.FileSystem implementation

func (fs *filesystem) StatFS(
	ctx context.Context,
	op *fuseops.StatFSOp) error {

	fs.Lock()
	defer fs.Unlock()

	log.Printf("StatFS")

	// Simulate a large amount of free space so that the Finder doesn't refuse to
	// copy in files. (See issue #125.) Use 2^17 as the block size because that
	// is the largest that OS X will pass on.
	op.BlockSize = 1 << 17
	op.Blocks = 1 << 33
	op.BlocksFree = op.Blocks
	op.BlocksAvailable = op.Blocks

	// Similarly with inodes.
	op.Inodes = 1 << 50
	op.InodesFree = op.Inodes

	// Prefer large transfers. This is the largest value that OS X will
	// faithfully pass on, according to fuseops/ops.go.
	op.IoSize = 1 << 20

	return nil
}

func (fs *filesystem) LookUpInode(ctx context.Context, op *fuseops.LookUpInodeOp) error {
	fs.Lock()
	defer fs.Unlock()

	log.Printf("LookUpInode[Parent: %v, Name: %s", op.Parent, op.Name)
	in, ok := fs.inodes[op.Parent]
	if !ok {
		return fuse.ENOENT
	}

	parent, ok := in.(inode.DirInode)
	if !ok {
		return fuse.EINVAL
	}

	child := parent.LookUpChild(ctx, op.Name)
	if child == nil {
		return fuse.ENOENT
	}

	if child.InodeID() < fuseops.RootInodeID {
		child.SetInodeID(fs.nextInodeID())
		fs.inodes[child.InodeID()] = child
	}

	op.Entry = fuseops.ChildInodeEntry{
		Child:      child.InodeID(),
		Attributes: *child.GetAttributes(),
	}

	return nil
}

func (fs *filesystem) GetInodeAttributes(ctx context.Context, op *fuseops.GetInodeAttributesOp) error {
	fs.Lock()
	defer fs.Unlock()

	log.Printf("GetInodeAttributes[InodeID: %v]", op.Inode)
	in, ok := fs.inodes[op.Inode]
	if !ok {
		return fuse.ENOENT
	}

	op.Attributes = *in.GetAttributes()

	return nil
}

func (fs *filesystem) SetInodeAttributes(ctx context.Context, op *fuseops.SetInodeAttributesOp) error {
	fs.Lock()
	defer fs.Unlock()

	log.Printf("SetInodeAttributes[Inode: %v]", op.Inode)

	in, ok := fs.inodes[op.Inode]
	if !ok {
		return fuse.ENOENT
	}

	attrs := in.GetAttributes()

	if op.Size != nil {
		attrs.Size = *op.Size
	}
	if op.Mode != nil {
		attrs.Mode = *op.Mode
	}
	if op.Atime != nil {
		attrs.Atime = *op.Atime
	}
	if op.Mtime != nil {
		attrs.Atime = *op.Mtime
	}

	op.Attributes = *attrs
	op.AttributesExpiration = time.Now().Add(time.Second * 10) // TODO remove hardcoding

	return nil
}

func (fs *filesystem) ForgetInode(ctx context.Context, op *fuseops.ForgetInodeOp) error {
	fs.Lock()
	defer fs.Unlock()

	log.Printf("ForgetInode[InodeID: %v, N: %v]", op.Inode, op.N)

	in, ok := fs.inodes[op.Inode]
	if !ok {
		return nil
	}

	remotePath := in.RemotePath()

	switch in.(type) {
	case inode.FileInode:
		if err := fs.sftpClient.Remove(remotePath); err != nil {
			log.Printf("failed to delete remote file '%s': %v", remotePath, err)
			return fuse.EIO
		}
	case inode.DirInode:
		if err := fs.sftpClient.RemoveDirectory(remotePath); err != nil {
			log.Printf("failed to delete remote dir '%s': %v", remotePath, err)
			return fuse.EIO
		}
	}

	return nil
}

func (fs *filesystem) BatchForget(context.Context, *fuseops.BatchForgetOp) error {
	fs.Lock()
	defer fs.Unlock()

	log.Println("BatchForget")
	return fuse.ENOSYS
}

func (fs *filesystem) MkDir(ctx context.Context, op *fuseops.MkDirOp) error {
	fs.Lock()
	defer fs.Unlock()

	log.Printf("MkDir[Parent: %v, Name: %v, Mode: %v]", op.Parent, op.Name, op.Mode)

	in, ok := fs.inodes[op.Parent]
	if !ok {
		return fuse.ENOENT
	}

	parent, ok := in.(inode.DirInode)
	if !ok {
		return fuse.EINVAL
	}

	if in := parent.LookUpChild(ctx, op.Name); in != nil {
		return fuse.EEXIST
	}

	attrs := fuseops.InodeAttributes{
		Size:  4096,
		Nlink: 1,
		Uid:   fs.uid,
		Gid:   fs.gid,
		Mode:  op.Mode,

		Atime:  time.Now(),
		Ctime:  time.Now(),
		Mtime:  time.Now(),
		Crtime: time.Now(),
	}

	remotePath := path.Join(parent.RemotePath(), op.Name)

	if err := fs.sftpClient.Mkdir(remotePath); err != nil {
		return fuse.EIO
	}

	dnode := inode.NewDir(fs.nextInodeID(), &attrs, remotePath, fs.sftpClient)
	fs.inodes[dnode.InodeID()] = dnode
	parent.AddEntry(dnode.Name(), dnode)

	op.Entry = fuseops.ChildInodeEntry{
		Child:      dnode.InodeID(),
		Attributes: attrs,
	}

	return nil
}

func (fs *filesystem) MkNode(context.Context, *fuseops.MkNodeOp) error {
	fs.Lock()
	defer fs.Unlock()

	log.Println("MkNode")
	return fuse.ENOSYS
}

func (fs *filesystem) CreateFile(ctx context.Context, op *fuseops.CreateFileOp) error {
	fs.Lock()
	defer fs.Unlock()

	log.Printf("CreateFile[Parent: %v, Name: %v]", op.Parent, op.Name)

	in, ok := fs.inodes[op.Parent]
	if !ok {
		return fuse.ENOENT
	}

	parent, ok := in.(inode.DirInode)
	if !ok {
		return fuse.EINVAL
	}

	if in := parent.LookUpChild(ctx, op.Name); in != nil {
		return fuse.EEXIST
	}

	attrs := fuseops.InodeAttributes{
		Size:  0,
		Nlink: 1,
		Uid:   fs.uid,
		Gid:   fs.gid,
		Mode:  op.Mode,

		Atime:  time.Now(),
		Ctime:  time.Now(),
		Mtime:  time.Now(),
		Crtime: time.Now(),
	}

	remotePath := path.Join(parent.RemotePath(), op.Name)

	fnode := inode.NewFile(fs.nextInodeID(), &attrs, remotePath)

	f, err := fs.sftpClient.OpenFile(fnode.RemotePath(), os.O_CREATE|os.O_RDWR)
	if err != nil {
		log.Printf("failed to open remote file: %v", err)
		return fuse.EIO
	}

	parent.AddEntry(fnode.Name(), fnode)
	fs.inodes[fnode.InodeID()] = fnode

	op.Handle = fs.nextHandleID()
	fs.handles[op.Handle] = handle.NewFileHandle(fnode.(inode.FileInode), f)

	op.Entry = fuseops.ChildInodeEntry{
		Child:      fnode.InodeID(),
		Attributes: attrs,
	}

	return nil
}

func (fs *filesystem) CreateLink(context.Context, *fuseops.CreateLinkOp) error {
	fs.Lock()
	defer fs.Unlock()

	log.Println("CreateLink")
	return fuse.ENOSYS
}

func (fs *filesystem) CreateSymlink(context.Context, *fuseops.CreateSymlinkOp) error {
	fs.Lock()
	defer fs.Unlock()

	log.Println("CreateSymlink")
	return fuse.ENOSYS
}

func (fs *filesystem) Rename(context.Context, *fuseops.RenameOp) error {
	fs.Lock()
	defer fs.Unlock()

	log.Println("Rename")
	return fuse.ENOSYS
}

func (fs *filesystem) RmDir(ctx context.Context, op *fuseops.RmDirOp) error {
	fs.Lock()
	defer fs.Unlock()

	log.Printf("RmDir[Parent: %v, Name: %v]", op.Parent, op.Name)

	in, ok := fs.inodes[op.Parent]
	if !ok {
		return fuse.ENOENT
	}

	parent, ok := in.(inode.DirInode)
	if !ok {
		return fuse.EINVAL
	}

	child := parent.LookUpChild(ctx, op.Name)
	if child == nil {
		return fuse.ENOENT
	}

	dnode, ok := child.(inode.DirInode)
	if !ok {
		return fuse.ENOTDIR
	}
	entries, err := dnode.GetEntries(ctx)
	if err != nil {
		return fuse.EIO
	}
	if len(entries) > 0 {
		return fuse.ENOTEMPTY
	}

	parent.RemoveEntry(op.Name)

	return nil
}

func (fs *filesystem) Unlink(ctx context.Context, op *fuseops.UnlinkOp) error {
	fs.Lock()
	defer fs.Unlock()

	log.Printf("Unlink[Parent: %v, Name: %v]", op.Parent, op.Name)
	in, ok := fs.inodes[op.Parent]
	if !ok {
		return fuse.ENOENT
	}

	parent, ok := in.(inode.DirInode)
	if !ok {
		return fuse.EINVAL
	}

	if c := parent.LookUpChild(ctx, op.Name); c == nil {
		return fuse.ENOENT
	} else {
		c.GetAttributes().Nlink--
	}

	parent.RemoveEntry(op.Name)

	return nil
}

// DIRECTORY OPS

// OpenDir ...
func (fs *filesystem) OpenDir(ctx context.Context, op *fuseops.OpenDirOp) error {
	fs.Lock()
	defer fs.Unlock()

	log.Printf("OpenDir[InodeID: %v]", op.Inode)

	node, ok := fs.inodes[op.Inode]
	if !ok {
		return fuse.ENOENT
	}

	dirInode, ok := node.(inode.DirInode)
	if !ok {
		return fuse.EINVAL
	}

	op.Handle = fs.nextHandleID()
	fs.handles[op.Handle] = handle.NewDirHandle(dirInode)

	return nil
}

// ReadDir ...
func (fs *filesystem) ReadDir(ctx context.Context, op *fuseops.ReadDirOp) error {
	fs.Lock()
	defer fs.Unlock()

	log.Printf("ReadDir[InodeID: %v, HandleID: %v]", op.Inode, op.Handle)

	_, ok := fs.inodes[op.Inode]
	if !ok {
		return fuse.ENOENT
	}

	handl, ok := fs.handles[op.Handle]
	if !ok {
		return fuse.EINVAL
	}

	dirHandle, ok := handl.(handle.DirHandle)
	if !ok {
		return fuse.EINVAL
	}

	if err := dirHandle.ReadDir(ctx, op); err != nil {
		return fuse.EIO
	}

	return nil
}

// ReleaseDirHandle ...
func (fs *filesystem) ReleaseDirHandle(ctx context.Context, op *fuseops.ReleaseDirHandleOp) error {
	fs.Lock()
	defer fs.Unlock()

	log.Printf("ReleaseDirHandle[HandleID: %v]", op.Handle)

	if _, ok := fs.handles[op.Handle]; !ok {
		return fuse.EINVAL
	}

	delete(fs.handles, op.Handle)

	return nil
}

// FILE OPS

// OpenFile ...
func (fs *filesystem) OpenFile(ctx context.Context, op *fuseops.OpenFileOp) error {
	fs.Lock()
	defer fs.Unlock()

	log.Printf("OpenFile[Inode: %v, Handle: %v]", op.Inode, op.Handle)
	in, ok := fs.inodes[op.Inode]
	if !ok {
		return fuse.ENOENT
	}

	fnode, ok := in.(inode.FileInode)
	if !ok {
		return fuse.EIO
	}

	remotePath := fnode.RemotePath()
	f, err := fs.sftpClient.OpenFile(remotePath, os.O_RDWR)
	if err != nil {
		log.Printf("failed to open remote file '%v': %v", remotePath, err)
		return fuse.EIO
	}

	op.Handle = fs.nextHandleID()
	fs.handles[op.Handle] = handle.NewFileHandle(fnode, f)

	return nil
}

// ReadFile ...
func (fs *filesystem) ReadFile(ctx context.Context, op *fuseops.ReadFileOp) error {
	fs.Lock()
	defer fs.Unlock()

	log.Printf("ReadFile[InodeID: %v, HandleID: %v]", op.Inode, op.Handle)
	_, ok := fs.inodes[op.Inode]
	if !ok {
		return fuse.ENOENT
	}

	handl, ok := fs.handles[op.Handle]
	if !ok {
		log.Println("invalid arg - no handle found")
		return fuse.EINVAL
	}

	fileHandle, ok := handl.(handle.FileHandle)
	if !ok {
		log.Println("invalid arg - not a file handle")
		return fuse.EINVAL
	}

	if err := fileHandle.ReadFile(ctx, op); err != nil {
		log.Printf("read file failed: %v", err)
		return fuse.EIO
	}

	return nil
}

// WriteFile ...
func (fs *filesystem) WriteFile(ctx context.Context, op *fuseops.WriteFileOp) error {
	fs.Lock()
	defer fs.Unlock()

	log.Printf("WriteFile[InodeID: %v, HandleID: %v]", op.Inode, op.Handle)
	_, ok := fs.inodes[op.Inode]
	if !ok {
		return fuse.ENOENT
	}

	handl, ok := fs.handles[op.Handle]
	if !ok {
		log.Println("invalid arg - no handle found")
		return fuse.EINVAL
	}

	fileHandle, ok := handl.(handle.FileHandle)
	if !ok {
		log.Println("invalid arg - not a file handle")
		return fuse.EINVAL
	}

	if err := fileHandle.WriteFile(ctx, op); err != nil {
		log.Printf("write file failed: %v", err)
		return fuse.EIO
	}

	return nil
}

// SyncFile ...
func (fs *filesystem) SyncFile(ctx context.Context, op *fuseops.SyncFileOp) error {
	fs.Lock()
	defer fs.Unlock()

	log.Printf("SyncFile[InodeID: %v, HandleID: %v]", op.Inode, op.Handle)
	return nil // TODO implement proper
}

// FlushFile ...
func (fs *filesystem) FlushFile(ctx context.Context, op *fuseops.FlushFileOp) error {
	fs.Lock()
	defer fs.Unlock()

	log.Printf("FlushFile[InodeID: %v, HandleID: %v]", op.Inode, op.Handle)
	return nil // TODO implement proper
}

// ReleaseFileHandle ...
func (fs *filesystem) ReleaseFileHandle(ctx context.Context, op *fuseops.ReleaseFileHandleOp) error {
	fs.Lock()
	defer fs.Unlock()

	log.Printf("ReleaseFileHandle[Handle: %v]", op.Handle)

	h, ok := fs.handles[op.Handle]
	if !ok {
		return fuse.EINVAL
	}

	fhandle, ok := h.(handle.FileHandle)
	if !ok {
		return fuse.EINVAL
	}

	if err := fhandle.CloseRemoteFile(); err != nil {
		log.Printf("failed to close remote file: %v", err)
	}

	delete(fs.handles, op.Handle)

	return nil
}

// MISC OPS

func (fs *filesystem) ReadSymlink(context.Context, *fuseops.ReadSymlinkOp) error {
	fs.Lock()
	defer fs.Unlock()

	log.Println("ReadSymlink")
	return fuse.ENOSYS
}

func (fs *filesystem) RemoveXattr(context.Context, *fuseops.RemoveXattrOp) error {
	fs.Lock()
	defer fs.Unlock()

	log.Println("RemoveXattr")
	return fuse.ENOSYS
}
func (fs *filesystem) GetXattr(context.Context, *fuseops.GetXattrOp) error {
	fs.Lock()
	defer fs.Unlock()

	log.Println("GetXattr")
	return fuse.ENOSYS
}
func (fs *filesystem) ListXattr(context.Context, *fuseops.ListXattrOp) error {
	fs.Lock()
	defer fs.Unlock()

	log.Println("ListXattr")
	return fuse.ENOSYS
}
func (fs *filesystem) SetXattr(context.Context, *fuseops.SetXattrOp) error {
	fs.Lock()
	defer fs.Unlock()

	log.Println("SetXattr")
	return fuse.ENOSYS
}
func (fs *filesystem) Fallocate(context.Context, *fuseops.FallocateOp) error {
	log.Println("Fallocate")
	return fuse.ENOSYS
}

// decremented to zero, and clean up any resources associated with the file
// system. No further calls to the file system will be made.
func (fs *filesystem) Destroy() {
	fs.Lock()
	defer fs.Unlock()

	log.Println("Destroy")
}
