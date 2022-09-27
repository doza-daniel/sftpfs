package inode

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/jacobsa/fuse/fuseops"
	"github.com/pkg/sftp"
)

type DirInode interface {
	Inode
	LookUpChild(ctx context.Context, name string) Inode
	GetEntries(ctx context.Context) ([]Inode, error)
	AddEntry(name string, in Inode)
	RemoveEntry(name string)
}

type dirInode struct {
	id         fuseops.InodeID
	attrs      *fuseops.InodeAttributes
	remotePath string
	entries    map[string]Inode

	sftpc    *sftp.Client
	upToDate bool
}

func NewDir(
	id fuseops.InodeID,
	attrs *fuseops.InodeAttributes,
	remotePath string,
	sftpc *sftp.Client,
) Inode {
	dir := &dirInode{
		id:         id,
		attrs:      attrs,
		remotePath: remotePath,
		entries:    make(map[string]Inode),

		sftpc:    sftpc,
		upToDate: false,
	}

	return dir
}

func (dir *dirInode) InodeID() fuseops.InodeID {
	return dir.id
}

func (dir *dirInode) SetInodeID(id fuseops.InodeID) {
	dir.id = id
}

func (dir *dirInode) GetAttributes() *fuseops.InodeAttributes {
	return dir.attrs
}

func (dir *dirInode) Name() string {
	return path.Base(dir.remotePath)
}

func (dir *dirInode) RemotePath() string {
	return dir.remotePath
}

func (dir *dirInode) AddEntry(name string, in Inode) {
	dir.entries[name] = in
}

func (dir *dirInode) RemoveEntry(name string) {
	delete(dir.entries, name)
}

func (dir *dirInode) LookUpChild(ctx context.Context, name string) Inode {
	if err := dir.populate(); err != nil {
		// TODO handle this gracefully
		panic(err)
	}

	return dir.entries[name] // TODO map this to not exists if no key present
}

func (dir *dirInode) GetEntries(ctx context.Context) ([]Inode, error) {
	if err := dir.populate(); err != nil {
		return nil, err
	}

	all := make([]Inode, len(dir.entries))

	i := 0
	for _, inode := range dir.entries {
		all[i] = inode
		i++
	}

	return all, nil
}

func (dir *dirInode) populate() (err error) {
	if dir.upToDate {
		return nil
	}

	newInodes := make(map[string]Inode)
	defer func() {
		if err != nil {
			err = fmt.Errorf("failed to populate '%s': %v", dir.remotePath, err)
			return
		}

		dir.upToDate = true
		for k, v := range newInodes {
			dir.AddEntry(k, v)
		}
	}()

	entries, err := dir.sftpc.ReadDir(dir.remotePath)
	if err != nil {
		return fmt.Errorf("failed to list remote dir '%s': %v", dir.remotePath, err)
	}

	for _, entry := range entries {
		in := dir.inodeFromRemoteDentry(entry)
		newInodes[in.Name()] = in
	}

	return nil
}

func (dir *dirInode) inodeFromRemoteDentry(entry os.FileInfo) Inode {
	attrs := fuseops.InodeAttributes{
		Size:  uint64(entry.Size()),
		Nlink: 1,
		Mode:  entry.Mode(),
		Mtime: entry.ModTime(),
	}

	remotePath := path.Join(dir.remotePath, entry.Name())

	if entry.IsDir() {
		return NewDir(0, &attrs, remotePath, dir.sftpc)
	}

	return NewFile(0, &attrs, remotePath)
}
