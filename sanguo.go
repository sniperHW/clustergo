package sanguo

import (
	"errors"
	"net"

	"github.com/sniperHW/sanguo/addr"
	"google.golang.org/protobuf/proto"
)

var (
	ErrInvaildNode   = errors.New("invaild node")
	ErrDuplicateConn = errors.New("duplicate node connection")
	ErrDial          = errors.New("dial failed")
	ErrAuth          = errors.New("auth failed")
)

type Sanguo struct {
	localAddr addr.Addr
	l         net.Listener
	nodeCache nodeCache
}

func (s *Sanguo) SendMessage(peer addr.LogicAddr, msg proto.Message) {

	//this.Post(peer, ss.NewMessage(msg, peer, this.serverState.selfAddr.Logic))
}
