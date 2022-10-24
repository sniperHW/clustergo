package ss

import (
	"encoding/binary"

	"github.com/sniperHW/rpcgo"
	"github.com/sniperHW/sanguo/addr"
	"github.com/sniperHW/sanguo/codec"
	"github.com/sniperHW/sanguo/codec/buffer"
	"google.golang.org/protobuf/proto"
)

const (
	sizeLen       = 4
	sizeFlag      = 1
	sizeToAndFrom = 8
	sizeCmd       = 2
	sizeRpcSeqNo  = 8
	minSize       = sizeLen + sizeFlag + sizeToAndFrom
)

const (
	Msg             = 0x8  //普通消息
	RpcReq          = 0x10 //RPC请求
	RpcResp         = 0x18 //RPC响应
	MaskMessageType = 0x38
	//Compress        = 0x80
	MaxPacketSize = 1024 * 4
)

func setMsgType(flag *byte, tt byte) {
	if tt == Msg || tt == RpcReq || tt == RpcResp {
		*flag |= tt
	}
}

func getMsgType(flag byte) byte {
	return flag & MaskMessageType
}

type Message struct {
	cmd  uint16
	to   addr.LogicAddr
	from addr.LogicAddr
	data interface{}
}

func NewMessage(to addr.LogicAddr, from addr.LogicAddr, data interface{}, cmd ...uint16) *Message {
	msg := &Message{
		to:   to,
		from: from,
		data: data,
	}

	if len(cmd) > 0 {
		msg.cmd = cmd[0]
	}

	return msg
}

func (m *Message) Data() interface{} {
	return m.data
}

func (m *Message) Cmd() uint16 {
	return m.cmd
}

func (m *Message) From() addr.LogicAddr {
	return m.from
}

func (m *Message) To() addr.LogicAddr {
	return m.to
}

// 透传消息
type RelayMessage struct {
	to   addr.LogicAddr
	from addr.LogicAddr
	data []byte
}

func (m *RelayMessage) Data() []byte {
	return m.data
}

func (m *RelayMessage) From() addr.LogicAddr {
	return m.from
}

func (m *RelayMessage) To() addr.LogicAddr {
	return m.to
}

func (m *RelayMessage) GetRpcRequest() *rpcgo.RequestMsg {
	if getMsgType(m.Data()[0]) != RpcReq {
		return nil
	} else {
		var req codec.RpcRequest
		if err := proto.Unmarshal(m.Data()[9:], &req); err != nil {
			return nil
		} else {
			return &rpcgo.RequestMsg{
				Seq:    req.Seq,
				Method: req.Method,
				Arg:    req.Arg,
				Oneway: req.Oneway,
			}
		}
	}
}

func (m *RelayMessage) ResetTo(to addr.LogicAddr) {
	m.to = to
	binary.BigEndian.PutUint32(m.data[1:], uint32(to))
}

func NewRelayMessage(to addr.LogicAddr, from addr.LogicAddr, data []byte) *RelayMessage {
	m := &RelayMessage{
		to:   to,
		from: from,
	}

	b := make([]byte, 0, len(data)+sizeLen)
	b = buffer.AppendUint32(b, uint32(len(data)))
	b = buffer.AppendBytes(b, data)
	m.data = b
	return m
}
