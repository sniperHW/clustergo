package ss

import (
	"fmt"
	"net"

	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/codec"
	"github.com/sniperHW/clustergo/codec/buffer"
	"github.com/sniperHW/clustergo/codec/pb"
	"github.com/sniperHW/rpcgo"
	"google.golang.org/protobuf/proto"
)

const Namespace string = "ss"

type SSCodec struct {
	codec.LengthPayloadPacketReceiver
	selfAddr addr.LogicAddr
	reader   buffer.BufferReader
}

func NewCodec(selfAddr addr.LogicAddr) *SSCodec {
	return &SSCodec{
		LengthPayloadPacketReceiver: codec.LengthPayloadPacketReceiver{
			Buff:          make([]byte, 4096),
			MaxPacketSize: MaxPacketSize,
		},
		selfAddr: selfAddr,
	}
}

func (ss *SSCodec) Encode(buffs net.Buffers, o interface{}) (net.Buffers, int) {
	switch o := o.(type) {
	case *Message:
		var data []byte
		var cmd uint32
		var err error

		flag := byte(0)

		switch msg := o.Payload().(type) {
		case proto.Message:
			if data, cmd, err = pb.Marshal(Namespace, msg); err != nil {
				return buffs, 0
			}

			payloadLen := sizeFlag + sizeToAndFrom + sizeCmd + len(data)

			totalLen := sizeLen + payloadLen

			if totalLen > MaxPacketSize {
				return buffs, 0
			}

			b := make([]byte, 0, totalLen-len(data))

			//写payload大小
			b = buffer.AppendInt(b, payloadLen)

			//设置普通消息标记
			setMsgType(&flag, Msg)
			//写flag
			b = buffer.AppendByte(b, flag)

			b = buffer.AppendUint32(b, uint32(o.To()))
			b = buffer.AppendUint32(b, uint32(o.From()))

			//写cmd
			b = buffer.AppendUint16(b, uint16(cmd))

			return append(buffs, b, data), totalLen
		case *rpcgo.RequestMsg:
			if data, err = rpcgo.EncodeRequest(msg); err != nil {
				return buffs, 0
			}

			payloadLen := sizeFlag + sizeToAndFrom + len(data)

			totalLen := sizeLen + payloadLen

			if totalLen > MaxPacketSize {
				return buffs, 0
			}

			b := make([]byte, 0, totalLen-len(data))

			//写payload大小
			b = buffer.AppendInt(b, payloadLen)

			//设置RPC请求标记
			setMsgType(&flag, RpcReq)

			//写flag
			b = buffer.AppendByte(b, flag)
			b = buffer.AppendUint32(b, uint32(o.To()))
			b = buffer.AppendUint32(b, uint32(o.From()))

			return append(buffs, b, data), totalLen
		case *rpcgo.ResponseMsg:
			if data, err = rpcgo.EncodeResponse(msg); err != nil {
				return buffs, 0
			}

			payloadLen := sizeFlag + sizeToAndFrom + len(data)

			totalLen := sizeLen + payloadLen

			if totalLen > MaxPacketSize {
				return buffs, 0
			}

			b := make([]byte, 0, totalLen-len(data))

			//写payload大小
			b = buffer.AppendInt(b, payloadLen)

			//设置RPC响应标记
			setMsgType(&flag, RpcResp)
			//写flag
			b = buffer.AppendByte(b, flag)

			b = buffer.AppendUint32(b, uint32(o.To()))
			b = buffer.AppendUint32(b, uint32(o.From()))

			return append(buffs, b, data), totalLen
		}
		return buffs, 0
	case *RelayMessage:
		return append(buffs, o.Payload()), len(o.Payload())
	default:
		return buffs, 0
	}
}

func (ss *SSCodec) isTarget(to addr.LogicAddr) bool {
	return ss.selfAddr == to
}

func (ss *SSCodec) Decode(payload []byte) (interface{}, error) {
	ss.reader.Reset(payload)
	flag := ss.reader.GetByte()
	to := addr.LogicAddr(ss.reader.GetUint32())
	from := addr.LogicAddr(ss.reader.GetUint32())

	if ss.isTarget(to) {
		//当前节点是数据包的目标接收方
		switch getMsgType(flag) {
		case Msg:
			cmd := ss.reader.GetUint16()
			data := ss.reader.GetAll()
			if msg, err := pb.Unmarshal(Namespace, uint32(cmd), data); err != nil {
				return nil, err
			} else {
				return NewMessage(to, from, msg, cmd), nil
			}
		case RpcReq:
			if req, err := rpcgo.DecodeRequest(ss.reader.GetAll()); err != nil {
				return nil, err
			} else {
				return NewMessage(to, from, req), nil
			}
		case RpcResp:
			if resp, err := rpcgo.DecodeResponse(ss.reader.GetAll()); err != nil {
				return nil, err
			} else {
				return NewMessage(to, from, resp), nil
			}
		default:
			return nil, fmt.Errorf("invaild packet type")
		}
	} else {
		//当前接收方不是目标节点，返回RelayMessage
		return NewRelayMessage(to, from, payload), nil
	}
}
