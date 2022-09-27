package handle

import (
	"context"
	"sftpfs/inode"

	"github.com/jacobsa/fuse"
	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
)

type DirHandle interface {
	ReadDir(context.Context, *fuseops.ReadDirOp) error
}

func NewDirHandle(dirInode inode.DirInode) Handle {
	return &dirHandle{dirInode}
}

type dirHandle struct {
	dirInode inode.DirInode
}

func (dh *dirHandle) Inode() inode.Inode {
	return dh.dirInode
}

func (dh *dirHandle) ReadDir(ctx context.Context, op *fuseops.ReadDirOp) error {
	entries, err := dh.dirInode.GetEntries(ctx)
	if err != nil {
		return err
	}

	index := int(op.Offset)
	if index > len(entries) {
		return fuse.EINVAL
	}

	dirents := make([]fuseutil.Dirent, len(entries)) // TODO: add self and parent refs (. / ..)
	for i, entry := range entries {
		t := fuseutil.DT_Unknown
		if _, ok := entry.(inode.DirInode); ok {
			t = fuseutil.DT_Directory
		}

		if _, ok := entry.(inode.FileInode); ok {
			t = fuseutil.DT_File
		}

		dirents[i] = fuseutil.Dirent{
			Type:   t,
			Inode:  entry.InodeID(),
			Name:   entry.Name(),
			Offset: fuseops.DirOffset(i) + 1,
		}
	}

	// We copy out entries until we run out of entries or space.
	for i := index; i < len(dirents); i++ {
		n := fuseutil.WriteDirent(op.Dst[op.BytesRead:], dirents[i])
		if n == 0 {
			break
		}

		op.BytesRead += n
	}

	return nil
}
