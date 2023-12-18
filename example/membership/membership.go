package membership

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"sync"
	"time"

	"log"

	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/codec/buffer"
	"github.com/sniperHW/clustergo/membership"
	"github.com/sniperHW/netgo"
)

type Node struct {
	LogicAddr string
	NetAddr   string
	Export    bool
	Available bool
}

const (
	addNode   = 1
	remNode   = 2
	modNode   = 3
	nodeInfo  = 4
	subscribe = 5
)

type AddNode struct {
	Node Node
}

type RemNode struct {
	LogicAddr string
}

type ModNode struct {
	Node Node
}

type NodeInfo struct {
	Nodes []Node
}

type Subscribe struct {
}

type codec struct {
	buff   []byte
	w      int
	r      int
	reader buffer.BufferReader
}

func (cc *codec) Encode(buffs net.Buffers, o interface{}) (net.Buffers, int) {
	if buff, err := json.Marshal(o); err != nil {
		log.Println("json.Marshal error:", err)
		return buffs, 0
	} else {
		b := make([]byte, 0, 8)
		b = buffer.AppendUint32(b, uint32(len(buff)+4))
		switch o.(type) {
		case *AddNode:
			b = buffer.AppendUint32(b, addNode)
		case *RemNode:
			b = buffer.AppendUint32(b, remNode)
		case *ModNode:
			b = buffer.AppendUint32(b, modNode)
		case *NodeInfo:
			b = buffer.AppendUint32(b, nodeInfo)
		case *Subscribe:
			b = buffer.AppendUint32(b, subscribe)
		default:
			log.Println("invaild packet")
			return buffs, 0
		}
		return append(buffs, b, buff), len(b) + len(buff)
	}
}

func (cc *codec) read(readable netgo.ReadAble, deadline time.Time) (int, error) {
	if err := readable.SetReadDeadline(deadline); err != nil {
		return 0, err
	} else {
		return readable.Read(cc.buff[cc.w:])
	}
}

func (cc *codec) Recv(readable netgo.ReadAble, deadline time.Time) (pkt []byte, err error) {
	sizeLen := 4
	for {
		unpackSize := cc.w - cc.r
		if unpackSize >= sizeLen {
			cc.reader.Reset(cc.buff[cc.r:cc.w])
			payload := int(cc.reader.GetUint32())

			if payload == 0 {
				return nil, fmt.Errorf("zero payload")
			}

			totalSize := payload + sizeLen

			if totalSize <= unpackSize {
				cc.r += sizeLen
				pkt := cc.buff[cc.r : cc.r+payload]
				cc.r += payload
				if cc.r == cc.w {
					cc.r = 0
					cc.w = 0
				}
				return pkt, nil
			} else {
				if totalSize > cap(cc.buff) {
					buff := make([]byte, totalSize)
					copy(buff, cc.buff[cc.r:cc.w])
					cc.buff = buff
				} else {
					//空间足够容纳下一个包，
					copy(cc.buff, cc.buff[cc.r:cc.w])
				}
				cc.w = cc.w - cc.r
				cc.r = 0
			}
		} else if cc.r > 0 {
			copy(cc.buff, cc.buff[cc.r:cc.w])
			cc.w = cc.w - cc.r
			cc.r = 0
		}

		var n int
		n, err = cc.read(readable, deadline)
		//log.Println(n, err)
		if n > 0 {
			cc.w += n
		}
		if nil != err {
			return
		}
	}
}

func (cc *codec) Decode(payload []byte) (interface{}, error) {
	cc.reader.Reset(payload)
	cmd := cc.reader.GetUint32()
	//log.Println("Decode", cmd, len(payload))
	switch cmd {
	case addNode:
		o := &AddNode{}
		err := json.Unmarshal(payload[4:], o)
		return o, err
	case remNode:
		o := &RemNode{}
		err := json.Unmarshal(payload[4:], o)
		return o, err
	case modNode:
		o := &ModNode{}
		err := json.Unmarshal(payload[4:], o)
		return o, err
	case nodeInfo:
		o := &NodeInfo{}
		err := json.Unmarshal(payload[4:], o)
		return o, err
	case subscribe:
		o := &Subscribe{}
		err := json.Unmarshal(payload[4:], o)
		return o, err
	}
	return nil, errors.New("invaild object")
}

type memberShipSvr struct {
	sync.Mutex
	nodes   map[string]*Node
	clients map[*netgo.AsynSocket]struct{}
}

func NewServer() *memberShipSvr {
	return &memberShipSvr{
		nodes:   map[string]*Node{},
		clients: map[*netgo.AsynSocket]struct{}{},
	}
}

func (svr *memberShipSvr) pub(socket *netgo.AsynSocket) {
	var nodeinfo NodeInfo
	for _, v := range svr.nodes {
		nodeinfo.Nodes = append(nodeinfo.Nodes, *v)
	}

	if socket != nil {
		for s, _ := range svr.clients {
			s.Send(&nodeinfo)
		}
	} else {
		for s, _ := range svr.clients {
			if s == socket {
				s.Send(&nodeinfo)
			}
		}
	}
}

func (svr *memberShipSvr) Start(service string, config []*Node) error {
	for _, v := range config {
		svr.nodes[v.LogicAddr] = v
	}

	_, serve, err := netgo.ListenTCP("tcp", service, func(conn *net.TCPConn) {
		log.Println("new client")
		cc := &codec{
			buff: make([]byte, 65535),
		}
		netgo.NewAsynSocket(netgo.NewTcpSocket(conn, cc),
			netgo.AsynSocketOption{
				Codec:    cc,
				AutoRecv: true,
			}).SetCloseCallback(func(s *netgo.AsynSocket, _ error) {
			svr.Lock()
			delete(svr.clients, s)
			svr.Unlock()
		}).SetPacketHandler(func(_ context.Context, as *netgo.AsynSocket, packet interface{}) error {
			//log.Println("on packet", packet)
			switch packet := packet.(type) {
			case *AddNode:
				svr.Lock()
				defer svr.Unlock()
				if _, ok := svr.nodes[packet.Node.LogicAddr]; !ok {
					svr.nodes[packet.Node.LogicAddr] = &packet.Node
					svr.pub(nil)
				}
			case *RemNode:
				svr.Lock()
				defer svr.Unlock()
				if _, ok := svr.nodes[packet.LogicAddr]; ok {
					delete(svr.nodes, packet.LogicAddr)
					svr.pub(nil)
				}
			case *ModNode:
				svr.Lock()
				defer svr.Unlock()
				if _, ok := svr.nodes[packet.Node.LogicAddr]; ok {
					svr.nodes[packet.Node.LogicAddr] = &packet.Node
					svr.pub(nil)
				}
			case *Subscribe:
				//log.Println("on Subscribe")
				svr.Lock()
				defer svr.Unlock()
				svr.clients[as] = struct{}{}
				svr.pub(as)

			}
			return nil
		}).Recv()

	})

	if err != nil {
		return err
	} else {
		serve()
		return nil
	}
}

type memberShipCli struct {
	svrService string
	nodes      []membership.Node
}

func NewClient(svr string) *memberShipCli {
	return &memberShipCli{
		svrService: svr,
	}
}

func (c *memberShipCli) Close() {

}

// 订阅变更
func (c *memberShipCli) Subscribe(updateCB func(membership.MemberInfo)) error {
	dialer := &net.Dialer{}
	for {
		if conn, err := dialer.Dial("tcp", c.svrService); err == nil {
			cc := &codec{
				buff: make([]byte, 65535),
			}
			as := netgo.NewAsynSocket(netgo.NewTcpSocket(conn.(*net.TCPConn), cc),
				netgo.AsynSocketOption{
					Codec:    cc,
					AutoRecv: true,
				}).SetCloseCallback(func(_ *netgo.AsynSocket, _ error) {
				go c.Subscribe(updateCB)
			}).SetPacketHandler(func(_ context.Context, as *netgo.AsynSocket, packet interface{}) error {
				locals := c.nodes
				updates := []membership.Node{}
				for _, v := range packet.(*NodeInfo).Nodes {
					if address, err := addr.MakeAddr(v.LogicAddr, v.NetAddr); err == nil {
						updates = append(updates, membership.Node{
							Export:    v.Export,
							Available: v.Available,
							Addr:      address,
						})
					}
				}

				sort.Slice(updates, func(l int, r int) bool {
					return updates[l].Addr.LogicAddr() < updates[r].Addr.LogicAddr()
				})

				var nodeInfo membership.MemberInfo

				i := 0
				j := 0

				for i < len(updates) && j < len(locals) {
					nodej := locals[j]
					nodei := updates[i]

					if nodei.Addr.LogicAddr() == nodej.Addr.LogicAddr() {
						if nodei.Addr.NetAddr().String() != nodej.Addr.NetAddr().String() ||
							nodei.Available != nodej.Available {
							nodeInfo.Update = append(nodeInfo.Update, nodei)
						}
						i++
						j++
					} else if nodei.Addr.LogicAddr() > nodej.Addr.LogicAddr() {
						//local  1 2 3 4 5 6
						//update 1 2 4 5 6
						//移除节点
						nodeInfo.Remove = append(nodeInfo.Remove, nodej)
						j++
					} else {
						//local  1 2 4 5 6
						//update 1 2 3 4 5 6
						//添加节点
						nodeInfo.Add = append(nodeInfo.Add, nodei)
						i++
					}
				}

				nodeInfo.Add = append(nodeInfo.Add, updates[i:]...)

				nodeInfo.Remove = append(nodeInfo.Remove, locals[j:]...)

				c.nodes = updates
				updateCB(nodeInfo)
				return nil
			}).Recv()
			as.Send(&Subscribe{})
			return nil
		} else {
			time.Sleep(time.Second)
		}
	}
}
