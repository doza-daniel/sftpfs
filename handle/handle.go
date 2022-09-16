package handle

import "sshfs/inode"

type Handle interface {
	Inode() inode.Inode
}
