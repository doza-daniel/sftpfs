package inode

import (
	"context"

	"github.com/jacobsa/fuse/fuseops"
)

type DirInode interface {
	Inode
	LookUpChild(ctx context.Context, name string) Inode
	GetEntries(ctx context.Context) ([]Inode, error)
	AddEntry(name string, in Inode)
	RemoveEntry(name string)
}

type dirInode struct {
	id      fuseops.InodeID
	attrs   *fuseops.InodeAttributes
	name    string
	entries map[string]Inode
}

func NewDir(id fuseops.InodeID, attrs *fuseops.InodeAttributes, name string) Inode {
	dir := &dirInode{
		id:      id,
		attrs:   attrs,
		name:    name,
		entries: make(map[string]Inode),
	}

	return dir
}

func (dir *dirInode) InodeID() fuseops.InodeID {
	return dir.id
}

func (dir *dirInode) GetAttributes() *fuseops.InodeAttributes {
	return dir.attrs
}

func (dir *dirInode) LookUpChild(ctx context.Context, name string) Inode {
	return dir.entries[name] // TODO map this to not exists if no key present
}

func (dir *dirInode) Name() string {
	return dir.name
}

func (dir *dirInode) GetEntries(ctx context.Context) ([]Inode, error) {
	all := make([]Inode, len(dir.entries))

	i := 0
	for _, inode := range dir.entries {
		all[i] = inode
		i++
	}

	return all, nil
}

func (dir *dirInode) AddEntry(name string, in Inode) {
	dir.entries[name] = in
}

func (dir *dirInode) RemoveEntry(name string) {
	delete(dir.entries, name)
}
