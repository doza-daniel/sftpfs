package handle

import (
	"context"
	"fmt"
	"io"
	"sshfs/inode"

	"github.com/jacobsa/fuse/fuseops"
	"github.com/pkg/sftp"
)

type FileHandle interface {
	ReadFile(context.Context, *fuseops.ReadFileOp) error
	WriteFile(context.Context, *fuseops.WriteFileOp) error
	CloseRemoteFile() error
}

func NewFileHandle(fnode inode.FileInode, file *sftp.File) Handle {
	return &fileHandle{file, fnode}
}

type fileHandle struct {
	file      *sftp.File
	fileInode inode.FileInode
}

func (fh *fileHandle) Inode() inode.Inode {
	return fh.fileInode
}

func (fh *fileHandle) ReadFile(_ context.Context, op *fuseops.ReadFileOp) error {
	n, err := fh.file.ReadAt(op.Dst, op.Offset)
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to read network file: %v", err)
	}

	op.BytesRead = n

	return nil
}

func (fh *fileHandle) WriteFile(_ context.Context, op *fuseops.WriteFileOp) error {
	if _, err := fh.file.WriteAt(op.Data, op.Offset); err != nil {
		return fmt.Errorf("failed to write to network file: %v", err)
	}

	return nil
}

func (fh *fileHandle) CloseRemoteFile() error {
	if err := fh.file.Close(); err != nil {
		return fmt.Errorf("failed to close remote file: %v", err)
	}

	return nil
}
