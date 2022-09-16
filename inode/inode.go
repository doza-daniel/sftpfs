package inode

import (
	"github.com/jacobsa/fuse/fuseops"
)

type Inode interface {
	InodeID() fuseops.InodeID
	Name() string
	GetAttributes() *fuseops.InodeAttributes
}
