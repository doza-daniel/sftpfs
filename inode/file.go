package inode

import (
	"fmt"
	"io"
	"path"
	"time"

	"github.com/jacobsa/fuse/fuseops"
)

type FileInode interface {
	Inode
	ReadAt(p []byte, off int64) (int, error)
	WriteAt(p []byte, off int64) (int, error)
}

type fileInode struct {
	id         fuseops.InodeID
	attrs      *fuseops.InodeAttributes
	remotePath string

	content []byte
}

func NewFile(id fuseops.InodeID, attrs *fuseops.InodeAttributes, remotePath string) Inode {
	return &fileInode{
		id:         id,
		attrs:      attrs,
		remotePath: remotePath,
	}
}

func (f *fileInode) InodeID() fuseops.InodeID {
	return f.id
}

func (f *fileInode) SetInodeID(id fuseops.InodeID) {
	f.id = id
}

func (f *fileInode) GetAttributes() *fuseops.InodeAttributes {
	return f.attrs
}

func (f *fileInode) Name() string {
	return path.Base(f.remotePath)
}

func (f *fileInode) RemotePath() string {
	return f.remotePath
}

func (f *fileInode) SetRemotePath(s string) {
	f.remotePath = s
}

func (f *fileInode) ReadAt(p []byte, off int64) (int, error) {
	// TODO fix this generated stuff
	if true {
		for i := range p {
			p[i] = byte(i%26 + 'a')
		}
		return len(p), nil
	}
	// Ensure the offset is in range.
	if off > int64(len(f.content)) {
		return 0, io.EOF
	}

	// Read what we can.
	n := copy(p, f.content[off:])
	if n < len(p) {
		return n, io.EOF
	}

	return n, nil
}

func (f *fileInode) WriteAt(p []byte, off int64) (int, error) {
	// Update the modification time.
	f.attrs.Mtime = time.Now()

	// Ensure that the contents slice is long enough.
	newLen := int(off) + len(p)
	if len(f.content) < newLen {
		padding := make([]byte, newLen-len(f.content))
		f.content = append(f.content, padding...)
		f.attrs.Size = uint64(newLen)
	}

	// Copy in the data.
	n := copy(f.content[off:], p)

	// Sanity check.
	if n != len(p) {
		panic(fmt.Sprintf("Unexpected short copy: %v", n))
	}

	return n, nil
}
