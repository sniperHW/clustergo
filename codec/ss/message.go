package ss

import (
	"encoding/binary"
	"reflect"

	"github.com/sniperHW/sanguo/addr"
	"github.com/sniperHW/sanguo/codec/buffer"
	"github.com/sniperHW/sanguo/codec/pb"
)

const (
	sizeLen      = 4
	sizeFlag     = 1
	sizeTo       = 4
	sizeFrom     = 4
	sizeCmd      = 2
	sizeRpcSeqNo = 8
	minSize      = sizeLen + sizeFlag
)

const (
	Relay           = 0x4  //跨集群透传消息
	Msg             = 0x8  //普通消息
	RpcReq          = 0x10 //RPC请求
	RpcResp         = 0x18 //RPC响应
	MaskMessageType = 0x38
	Compress        = 0x80
	MaxPacketSize   = 1024 * 4
)

func setCompressFlag(flag *byte) {
	*flag |= Compress
}

func getCompresFlag(flag byte) bool {
	return (flag & Compress) != 0
}

func setMsgType(flag *byte, tt byte) {
	if tt == Msg || tt == RpcReq || tt == RpcResp {
		*flag |= tt
	}
}

func getMsgType(flag byte) byte {
	return flag & MaskMessageType
}

func setRelay(flag *byte) {
	*flag |= Relay
}

func isRelay(flag byte) bool {
	return (flag & Relay) != 0
}

type Message struct {
	data      interface{}
	relayInfo []addr.LogicAddr
}

func NewMessage(data interface{}, relay ...addr.LogicAddr) *Message {
	m := &Message{
		data: data,
	}
	if len(relay) == 2 {
		m.relayInfo = relay
	}
	return m
}

func (this *Message) GetData() interface{} {
	return this.data
}

func (this *Message) GetCmd() uint16 {
	name := reflect.TypeOf(this.data).String()
	return uint16(pb.GetCmdByName(name))
}

func (this *Message) From() addr.LogicAddr {
	if len(this.relayInfo) == 2 {
		return this.relayInfo[1]
	} else {
		return addr.LogicAddr(0)
	}
}

// 透传消息，从To到From
type RelayMessage struct {
	To   addr.LogicAddr
	From addr.LogicAddr
	data []byte
}

func (this *RelayMessage) GetData() []byte {
	return this.data
}

func (this *RelayMessage) IsRPCReq() bool {
	return getMsgType(this.data[4]) == RpcReq
}

func (this *RelayMessage) ResetTo(to addr.LogicAddr) {
	this.To = to
	binary.BigEndian.PutUint32(this.data[5:5+4], uint32(to))
}

func NewRelayMessage(to addr.LogicAddr, from addr.LogicAddr, data []byte) *RelayMessage {
	m := &RelayMessage{
		To:   to,
		From: from,
		data: make([]byte, 0, len(data)+sizeLen),
	}
	m.data = buffer.AppendUint32(m.data, uint32(len(data)))
	m.data = buffer.AppendBytes(m.data, data)
	return m
}
