package sanguo

import (
	"errors"
	"net"

	"github.com/sniperHW/rpcgo"
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
	rpcSvr    *rpcgo.Server
	rpcCli    *rpcgo.Client
}

func (s *Sanguo) SendMessage(peer addr.LogicAddr, msg proto.Message) {
	if peer == s.localAddr.LogicAddr() {
		s.dispatchMessage(peer, msg)
	}
}

func (s *Sanguo) dispatchMessage(from addr.LogicAddr, msg proto.Message) {

}
