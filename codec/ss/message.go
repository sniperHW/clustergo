package ss

import (
	"encoding/binary"
	"github.com/sniperHW/sanguo/cluster/addr"
)

type Message struct {
	name      string
	data      interface{}
	relayInfo []addr.LogicAddr
}

func NewMessage(name string, data interface{}, relay ...addr.LogicAddr) *Message {
	m := &Message{
		name: name,
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

func (this *Message) GetName() string {
	return this.name
}

func (this *Message) From() addr.LogicAddr {
	if len(this.relayInfo) == 2 {
		return this.relayInfo[1]
	} else {
		return addr.LogicAddr(0)
	}
}

//透传消息，从To到From
type RelayMessage struct {
	To   addr.LogicAddr
	From addr.LogicAddr
	data []byte
}

func (this *RelayMessage) GetData() []byte {
	return this.data
}

func (this *RelayMessage) IsRPCReq() bool {
	return getMsgType(this.data[4]) == RPCREQ
}

func (this *RelayMessage) GetSeqno() uint64 {
	beg := sizeLen + sizeFlag + sizeTo + sizeFrom + sizeCmd
	end := beg + sizeRPCSeqNo
	return binary.BigEndian.Uint64(this.data[beg:end])
}

func NewRelayMessage(to addr.LogicAddr, from addr.LogicAddr, data []byte) *RelayMessage {
	m := &RelayMessage{
		To:   to,
		From: from,
		data: make([]byte, len(data)),
	}
	copy(m.data, data)
	return m
}

type RCPRelayErrorMessage struct {
	To     addr.LogicAddr
	From   addr.LogicAddr
	Seqno  uint64
	ErrMsg string
}
