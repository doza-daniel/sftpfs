package inode

import (
	"github.com/jacobsa/fuse/fuseops"
)

type Inode interface {
	InodeID() fuseops.InodeID
	Name() string
	RemotePath() string
	GetAttributes() *fuseops.InodeAttributes
}
