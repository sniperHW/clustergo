package sanguo

import (
	"container/list"
	"sync"

	"github.com/sniperHW/netgo"
	"github.com/sniperHW/sanguo/addr"
)

type ring struct {
	head int
	tail int
	size int
	data []interface{}
}

func (r *ring) push(v interface{}) {
	r.data[r.tail] = v
	r.tail = (r.tail + 1) % len(r.data)
	if r.size < len(r.data) {
		r.size++
	} else {
		r.head = (r.head + 1) % len(r.data)
	}
}

func (r *ring) pop() (interface{}, bool) {
	if r.size > 0 {
		v := r.data[r.head]
		r.data[r.head] = nil
		r.head = (r.head + 1) % len(r.data)
		r.size -= 1
		return v, true
	} else {
		return nil, false
	}
}

type node struct {
	sync.Mutex
	addr         addr.Addr
	dailing      bool
	socket       *netgo.AsynSocket
	pendingMsg   ring
	pendingCall  *list.List
	pendingReply ring
}

func (n node) isHarbor() bool {
	return n.addr.Logic.Type() == addr.HarbarType
}
