package inode

import (
	"github.com/jacobsa/fuse/fuseops"
)

type Inode interface {
	InodeID() fuseops.InodeID
	SetInodeID(fuseops.InodeID)
	Name() string
	RemotePath() string
	SetRemotePath(string)
	GetAttributes() *fuseops.InodeAttributes
}
