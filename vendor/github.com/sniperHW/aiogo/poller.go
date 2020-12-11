package aiogo

import (
	"sync"
)

const hashMask int = 8
const hashSize int = 1 << hashMask

type fd2Conn []sync.Map

func (self *fd2Conn) add(conn *Conn) {
	(*self)[conn.fd>>hashMask].Store(conn.fd, conn)
}

func (self *fd2Conn) get(fd int) (*Conn, bool) {
	v, ok := (*self)[fd>>hashMask].Load(fd)
	if ok {
		return v.(*Conn), true
	} else {
		return nil, false
	}
}

func (self *fd2Conn) remove(conn *Conn) {
	(*self)[conn.fd>>hashMask].Delete(conn.fd)
}

type pollerI interface {
	trigger() error
	watch(*Conn) bool
	unwatch(*Conn) bool
	wait(*int32)
	enableWrite(*Conn) bool
	disableWrite(*Conn) bool
}
