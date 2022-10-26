package ss

import (
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
	cmd     uint16
	to      addr.LogicAddr
	from    addr.LogicAddr
	payload interface{}
}

func NewMessage(to addr.LogicAddr, from addr.LogicAddr, payload interface{}, cmd ...uint16) *Message {
	msg := &Message{
		to:      to,
		from:    from,
		payload: payload,
	}

	if len(cmd) > 0 {
		msg.cmd = cmd[0]
	}

	return msg
}

func (m *Message) Payload() interface{} {
	return m.payload
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
	to      addr.LogicAddr
	from    addr.LogicAddr
	payload []byte
}

func (m *RelayMessage) Payload() []byte {
	return m.payload
}

func (m *RelayMessage) From() addr.LogicAddr {
	return m.from
}

func (m *RelayMessage) To() addr.LogicAddr {
	return m.to
}

func (m *RelayMessage) GetRpcRequest() *rpcgo.RequestMsg {
	if getMsgType(m.payload[4]) != RpcReq {
		return nil
	} else {
		var req codec.RpcRequest
		if err := proto.Unmarshal(m.payload[13:], &req); err != nil {
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

func NewRelayMessage(to addr.LogicAddr, from addr.LogicAddr, payload []byte) *RelayMessage {
	m := &RelayMessage{
		to:   to,
		from: from,
	}

	b := make([]byte, 0, len(payload)+sizeLen)
	b = buffer.AppendUint32(b, uint32(len(payload)))
	b = buffer.AppendBytes(b, payload)
	m.payload = b
	return m
}
