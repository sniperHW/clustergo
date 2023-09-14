package ss

import (
	"encoding/binary"
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
	pbMeta   *pb.PbMeta
}

func NewCodec(selfAddr addr.LogicAddr) *SSCodec {
	return &SSCodec{
		LengthPayloadPacketReceiver: codec.LengthPayloadPacketReceiver{
			Buff:          make([]byte, 4096),
			MaxPacketSize: MaxPacketSize,
		},
		selfAddr: selfAddr,
		pbMeta:   pb.GetMeta(Namespace),
	}
}

func (ss *SSCodec) encode(buffs net.Buffers, m *Message, cmd uint16, flag byte, data []byte) (net.Buffers, int) {
	payloadLen := sizeFlag + sizeToAndFrom + len(data)

	if cmd != 0 {
		payloadLen += sizeCmd
	}

	totalLen := sizeLen + payloadLen

	if totalLen > MaxPacketSize {
		return buffs, 0
	}

	b := make([]byte, 13, totalLen-len(data))

	//写payload大小

	//b = buffer.AppendInt(b, payloadLen)
	binary.BigEndian.PutUint32(b, uint32(payloadLen))

	//写flag
	//b = buffer.AppendByte(b, flag)
	b[4] = flag

	//b = buffer.AppendUint32(b, uint32(m.To()))
	//b = buffer.AppendUint32(b, uint32(m.From()))
	binary.BigEndian.PutUint32(b[5:], uint32(m.To()))
	binary.BigEndian.PutUint32(b[9:], uint32(m.From()))

	if cmd != 0 {
		//写cmd
		b = buffer.AppendUint16(b, cmd)
	}

	return append(buffs, b, data), totalLen
}

func (ss *SSCodec) Encode(buffs net.Buffers, o interface{}) (net.Buffers, int) {
	switch o := o.(type) {
	case *Message:
		var data []byte
		var err error

		flag := byte(0)

		switch msg := o.Payload().(type) {
		case []byte:
			if o.cmd == 0 || len(msg) == 0 {
				return buffs, 0
			}
			//设置Bin消息标记
			setMsgType(&flag, BinMsg)
			return ss.encode(buffs, o, o.cmd, flag, msg)
		case proto.Message:
			var cmd uint32
			if data, cmd, err = ss.pbMeta.Marshal(msg); err != nil {
				return buffs, 0
			}
			//设置Pb消息标记
			setMsgType(&flag, PbMsg)
			return ss.encode(buffs, o, uint16(cmd), flag, data)
		case *rpcgo.RequestMsg:
			//设置RPC请求标记
			setMsgType(&flag, RpcReq)
			return ss.encode(buffs, o, 0, flag, rpcgo.EncodeRequest(msg))
		case *rpcgo.ResponseMsg:
			//设置RPC响应标记
			setMsgType(&flag, RpcResp)
			return ss.encode(buffs, o, 0, flag, rpcgo.EncodeResponse(msg))
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
		case BinMsg:
			cmd := ss.reader.GetUint16()
			data := ss.reader.GetAll()
			return NewMessage(to, from, data, cmd), nil
		case PbMsg:
			cmd := ss.reader.GetUint16()
			data := ss.reader.GetAll()
			if msg, err := ss.pbMeta.Unmarshal(uint32(cmd), data); err != nil {
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
