package ss

import (
	"fmt"
	"net"

	"reflect"

	"github.com/sniperHW/rpcgo"
	"github.com/sniperHW/sanguo/addr"
	"github.com/sniperHW/sanguo/codec"
	"github.com/sniperHW/sanguo/codec/buffer"
	"github.com/sniperHW/sanguo/codec/pb"
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
		var pbbytes []byte
		var cmd uint32
		var err error

		flag := byte(0)

		switch msg := o.Payload().(type) {
		case proto.Message:
			if pbbytes, cmd, err = pb.Marshal(Namespace, msg); err != nil {
				return buffs, 0
			}

			payloadLen := sizeFlag + sizeToAndFrom + sizeCmd + len(pbbytes)

			totalLen := sizeLen + payloadLen

			if totalLen > MaxPacketSize {
				return buffs, 0
			}

			b := make([]byte, 0, totalLen-len(pbbytes))

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

			return append(buffs, b, pbbytes), totalLen
		case *rpcgo.RequestMsg:
			req := &codec.RpcRequest{
				Seq:    msg.Seq,
				Method: msg.Method,
				Arg:    msg.Arg,
				Oneway: msg.Oneway,
			}

			if pbbytes, err = proto.Marshal(req); err != nil {
				return buffs, 0
			}

			payloadLen := sizeFlag + sizeToAndFrom + len(pbbytes)

			totalLen := sizeLen + payloadLen

			if totalLen > MaxPacketSize {
				return buffs, 0
			}

			b := make([]byte, 0, totalLen-len(pbbytes))

			//写payload大小
			b = buffer.AppendInt(b, payloadLen)

			//设置RPC请求标记
			setMsgType(&flag, RpcReq)

			//写flag
			b = buffer.AppendByte(b, flag)
			b = buffer.AppendUint32(b, uint32(o.To()))
			b = buffer.AppendUint32(b, uint32(o.From()))

			return append(buffs, b, pbbytes), totalLen
		case *rpcgo.ResponseMsg:
			resp := &codec.RpcResponse{
				Seq: msg.Seq,
				Ret: msg.Ret,
			}

			if msg.Err != nil {
				resp.ErrCode = uint32(msg.Err.Code)
				resp.ErrDesc = msg.Err.Err
			}

			if pbbytes, err = proto.Marshal(resp); err != nil {
				panic(err)
				return buffs, 0
			}

			payloadLen := sizeFlag + sizeToAndFrom + len(pbbytes)

			totalLen := sizeLen + payloadLen

			if totalLen > MaxPacketSize {
				return buffs, 0
			}

			b := make([]byte, 0, totalLen-len(pbbytes))

			//写payload大小
			b = buffer.AppendInt(b, payloadLen)

			//设置RPC响应标记
			setMsgType(&flag, RpcResp)
			//写flag
			b = buffer.AppendByte(b, flag)

			b = buffer.AppendUint32(b, uint32(o.To()))
			b = buffer.AppendUint32(b, uint32(o.From()))

			return append(buffs, b, pbbytes), totalLen
		}
		return buffs, 0
	case *RelayMessage:
		return append(buffs, o.Payload()), len(o.Payload())
	default:
		panic(reflect.TypeOf(o).String())
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
			data := ss.reader.GetAll()
			var req codec.RpcRequest
			if err := proto.Unmarshal(data, &req); err != nil {
				return nil, err
			} else {
				return NewMessage(to, from, &rpcgo.RequestMsg{
					Seq:    req.Seq,
					Method: req.Method,
					Arg:    req.Arg,
					Oneway: req.Oneway,
				}), nil
			}
		case RpcResp:
			data := ss.reader.GetAll()
			var resp codec.RpcResponse
			if err := proto.Unmarshal(data, &resp); err != nil {
				return nil, err
			} else {
				r := &rpcgo.ResponseMsg{
					Seq: resp.Seq,
					Ret: resp.Ret,
				}

				if resp.ErrCode != 0 {
					r.Err = &rpcgo.Error{
						Code: int(resp.ErrCode),
						Err:  resp.ErrDesc,
					}
				}
				return NewMessage(to, from, r), nil
			}
		default:
			return nil, fmt.Errorf("invaild packet type")
		}
	} else {
		//当前接收方不是目标节点，返回RelayMessage
		return NewRelayMessage(to, from, payload), nil
	}
}
