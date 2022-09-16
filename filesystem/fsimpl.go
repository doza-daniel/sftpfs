package filesystem

import (
	"context"
	"log"
	"os"
	"sshfs/handle"
	"sshfs/inode"
	"time"

	"github.com/jacobsa/fuse"
	"github.com/jacobsa/fuse/fuseops"
)

// fuseutil.FileSystem implementation

func (fs *filesystem) StatFS(
	ctx context.Context,
	op *fuseops.StatFSOp) error {

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

	op.Entry = fuseops.ChildInodeEntry{
		Child:      child.InodeID(),
		Attributes: *child.GetAttributes(),
	}

	return nil
}

func (fs *filesystem) GetInodeAttributes(ctx context.Context, op *fuseops.GetInodeAttributesOp) error {
	log.Printf("GetInodeAttributes[InodeID: %v]", op.Inode)
	in, ok := fs.inodes[op.Inode]
	if !ok {
		return fuse.ENOENT
	}

	op.Attributes = *in.GetAttributes()
	op.AttributesExpiration = time.Now().Add(5 * time.Second) // TODO: hardcoded for now

	return nil
}

func (fs *filesystem) SetInodeAttributes(ctx context.Context, op *fuseops.SetInodeAttributesOp) error {
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
	log.Printf("ForgetInode[InodeID: %v, N: %v]", op.Inode, op.N)
	return nil // TODO: implement this
}

func (fs *filesystem) BatchForget(context.Context, *fuseops.BatchForgetOp) error {
	log.Println("BatchForget")
	return fuse.ENOSYS
}

func (fs *filesystem) MkDir(ctx context.Context, op *fuseops.MkDirOp) error {
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

	dnode := inode.NewDir(fs.nextInodeID(), &attrs, op.Name)
	fs.inodes[dnode.InodeID()] = dnode
	parent.AddEntry(dnode.Name(), dnode)

	op.Entry = fuseops.ChildInodeEntry{
		Child:      dnode.InodeID(),
		Attributes: attrs,
	}

	return nil
}

func (fs *filesystem) MkNode(context.Context, *fuseops.MkNodeOp) error {
	log.Println("MkNode")
	return fuse.ENOSYS
}

func (fs *filesystem) CreateFile(ctx context.Context, op *fuseops.CreateFileOp) error {
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

	fnode := inode.NewFile(fs.nextInodeID(), &attrs, op.Name)
	fs.inodes[fnode.InodeID()] = fnode

	parent.AddEntry(fnode.Name(), fnode)

	f, err := fs.sftpClient.Open("/home/mi13119/" + fnode.Name())
	if err != nil {
		log.Printf("failed to open remote file: %v", err)
		return fuse.EIO
	}

	op.Handle = fs.nextHandleID()
	fs.handles[op.Handle] = handle.NewFileHandle(fnode.(inode.FileInode), f)

	op.Entry = fuseops.ChildInodeEntry{
		Child:      fnode.InodeID(),
		Attributes: attrs,
	}

	return nil
}

func (fs *filesystem) CreateLink(context.Context, *fuseops.CreateLinkOp) error {
	log.Println("CreateLink")
	return fuse.ENOSYS
}

func (fs *filesystem) CreateSymlink(context.Context, *fuseops.CreateSymlinkOp) error {
	log.Println("CreateSymlink")
	return fuse.ENOSYS
}

func (fs *filesystem) Rename(context.Context, *fuseops.RenameOp) error {
	log.Println("Rename")
	return fuse.ENOSYS
}

func (fs *filesystem) RmDir(context.Context, *fuseops.RmDirOp) error {
	log.Println("RmDir")
	return fuse.ENOSYS
}

func (fs *filesystem) Unlink(ctx context.Context, op *fuseops.UnlinkOp) error {
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

	return dirHandle.ReadDir(ctx, op)
}

// ReleaseDirHandle ...
func (fs *filesystem) ReleaseDirHandle(ctx context.Context, op *fuseops.ReleaseDirHandleOp) error {
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
	log.Printf("OpenFile[Inode: %v, Handle: %v]", op.Inode, op.Handle)
	in, ok := fs.inodes[op.Inode]
	if !ok {
		return fuse.ENOENT
	}

	fnode, ok := in.(inode.FileInode)
	if !ok {
		return fuse.EIO
	}

	remotePath := "/home/mi13119/" + fnode.Name()
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
	log.Printf("SyncFile[InodeID: %v, HandleID: %v]", op.Inode, op.Handle)
	return nil // TODO implement proper
}

// FlushFile ...
func (fs *filesystem) FlushFile(ctx context.Context, op *fuseops.FlushFileOp) error {
	log.Printf("FlushFile[InodeID: %v, HandleID: %v]", op.Inode, op.Handle)
	return nil // TODO implement proper
}

// ReleaseFileHandle ...
func (fs *filesystem) ReleaseFileHandle(ctx context.Context, op *fuseops.ReleaseFileHandleOp) error {
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
	log.Println("ReadSymlink")
	return fuse.ENOSYS
}

func (fs *filesystem) RemoveXattr(context.Context, *fuseops.RemoveXattrOp) error {
	log.Println("RemoveXattr")
	return fuse.ENOSYS
}
func (fs *filesystem) GetXattr(context.Context, *fuseops.GetXattrOp) error {
	log.Println("GetXattr")
	return fuse.ENOSYS
}
func (fs *filesystem) ListXattr(context.Context, *fuseops.ListXattrOp) error {
	log.Println("ListXattr")
	return fuse.ENOSYS
}
func (fs *filesystem) SetXattr(context.Context, *fuseops.SetXattrOp) error {
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
	log.Println("Destroy")
}
