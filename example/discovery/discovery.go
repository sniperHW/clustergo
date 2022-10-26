package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"log"

	"github.com/sniperHW/netgo"
	"github.com/sniperHW/sanguo/addr"
	"github.com/sniperHW/sanguo/codec/buffer"
	"github.com/sniperHW/sanguo/discovery"
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
	//log.Println("Encode", o)
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
		//log.Println(b, buff, len(b)+len(buff))
		//buffs = append(buffs, b, buff)
		//log.Println(buffs)
		//return buffs, len(b) + len(buff)
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

type discoverySvr struct {
	sync.Mutex
	nodes   map[string]*Node
	clients map[*netgo.AsynSocket]struct{}
}

func NewServer() *discoverySvr {
	return &discoverySvr{
		nodes:   map[string]*Node{},
		clients: map[*netgo.AsynSocket]struct{}{},
	}
}

func (svr *discoverySvr) pub(socket *netgo.AsynSocket) {
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

func (svr *discoverySvr) Start(service string, config []*Node) error {
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

type discoverCli struct {
	svrService string
}

func NewClient(svr string) *discoverCli {
	return &discoverCli{
		svrService: svr,
	}
}

// 订阅变更
func (c *discoverCli) Subscribe(updateCB func([]discovery.Node)) error {
	dialer := &net.Dialer{}
	for {
		if conn, err := dialer.Dial("tcp", c.svrService); err == nil {
			//log.Println("dial discover ok")
			cc := &codec{
				buff: make([]byte, 65535),
			}
			as := netgo.NewAsynSocket(netgo.NewTcpSocket(conn.(*net.TCPConn), cc),
				netgo.AsynSocketOption{
					Codec:    cc,
					AutoRecv: true,
				}).SetCloseCallback(func(_ *netgo.AsynSocket, _ error) {
				//log.Println("socket close")
				go c.Subscribe(updateCB)
			}).SetPacketHandler(func(_ context.Context, as *netgo.AsynSocket, packet interface{}) error {
				nodes := []discovery.Node{}
				for _, v := range packet.(*NodeInfo).Nodes {
					if address, err := addr.MakeAddr(v.LogicAddr, v.NetAddr); err == nil {
						nodes = append(nodes, discovery.Node{
							Export:    v.Export,
							Available: v.Available,
							Addr:      address,
						})
					}
				}
				//log.Println("cli on packet", packet.(*NodeInfo).Nodes)
				updateCB(nodes)
				return nil
			}).Recv()
			as.Send(&Subscribe{})
			return nil
		} else {
			//log.Println("dial failed", err)
			time.Sleep(time.Second)
		}
	}
}
