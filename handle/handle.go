package handle

import "sftpfs/inode"

type Handle interface {
	Inode() inode.Inode
}
