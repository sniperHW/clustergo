package ss

import (
	"fmt"
	"net"
	"time"

	"github.com/sniperHW/netgo"
	"github.com/sniperHW/rpcgo"
	"github.com/sniperHW/sanguo/addr"
	"github.com/sniperHW/sanguo/codec"
	"github.com/sniperHW/sanguo/codec/buffer"
	"github.com/sniperHW/sanguo/codec/pb"
	"google.golang.org/protobuf/proto"
)

type SSCodec struct {
	buff     []byte
	w        int
	r        int
	selfAddr addr.LogicAddr
	reader   buffer.BufferReader
}

func NewCodec(selfAddr addr.LogicAddr) *SSCodec {
	return &SSCodec{
		selfAddr: selfAddr,
		buff:     make([]byte, 4096),
	}
}

func (ss *SSCodec) Encode(buffs net.Buffers, o interface{}) (net.Buffers, int) {
	switch o := o.(type) {
	case *Message:
		var pbbytes []byte
		var cmd uint32
		var err error

		flag := byte(0)

		switch msg := o.Data().(type) {
		case proto.Message:
			if pbbytes, cmd, err = pb.Marshal("ss", msg); err != nil {
				return buffs, 0
			}

			payloadLen := sizeFlag + sizeToAndFrom + sizeCmd + len(pbbytes)

			totalLen := sizeLen + payloadLen

			if totalLen > MaxPacketSize {
				return buffs, 0
			}

			b := make([]byte, totalLen)

			//写payload大小
			b = buffer.AppendInt(b, payloadLen)

			//设置普通消息标记
			setMsgType(&flag, Msg)
			//写flag
			b = buffer.AppendByte(b, flag)

			b = buffer.AppendUint32(b, uint32(o.To))
			b = buffer.AppendUint32(b, uint32(o.From))

			//写cmd
			b = buffer.AppendUint16(b, uint16(cmd))
			//写数据
			b = buffer.AppendBytes(b, pbbytes)

			return append(buffs, b), len(b)
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

			b := make([]byte, totalLen)

			//写payload大小
			b = buffer.AppendInt(b, payloadLen)

			//设置RPC请求标记
			setMsgType(&flag, RpcReq)

			//写flag
			b = buffer.AppendByte(b, flag)
			b = buffer.AppendUint32(b, uint32(o.To))
			b = buffer.AppendUint32(b, uint32(o.From))
			b = buffer.AppendBytes(b, pbbytes)

			return append(buffs, b), len(b)
		case *rpcgo.ResponseMsg:
			resp := &codec.RpcResponse{
				Seq:     msg.Seq,
				Ret:     msg.Ret,
				ErrCode: uint32(msg.Err.Code),
				ErrDesc: msg.Err.Err,
			}

			if pbbytes, err = proto.Marshal(resp); err != nil {
				return buffs, 0
			}

			payloadLen := sizeFlag + sizeToAndFrom + len(pbbytes)

			totalLen := sizeLen + payloadLen

			if totalLen > MaxPacketSize {
				return buffs, 0
			}

			b := make([]byte, totalLen)

			//写payload大小
			b = buffer.AppendInt(b, payloadLen)

			//设置RPC响应标记
			setMsgType(&flag, RpcResp)
			//写flag
			b = buffer.AppendByte(b, flag)

			b = buffer.AppendUint32(b, uint32(o.To))
			b = buffer.AppendUint32(b, uint32(o.From))

			b = buffer.AppendBytes(b, pbbytes)

			return append(buffs, b), len(b)
		}
		return buffs, 0
	case *RelayMessage:
		return append(buffs, o.Data()), len(o.Data())
	default:
		return buffs, 0
	}
}

func (ss *SSCodec) read(readable netgo.ReadAble, deadline time.Time) (int, error) {
	if err := readable.SetReadDeadline(deadline); err != nil {
		return 0, err
	} else {
		return readable.Read(ss.buff[ss.w:])
	}
}

func (ss *SSCodec) Recv(readable netgo.ReadAble, deadline time.Time) (pkt []byte, err error) {
	for {
		unpackSize := ss.w - ss.r
		if unpackSize >= minSize {
			ss.reader.Reset(ss.buff[ss.r:ss.w])
			payload := int(ss.reader.GetUint32())

			if payload == 0 {
				return nil, fmt.Errorf("zero payload")
			}

			totalSize := payload + sizeLen

			if totalSize > MaxPacketSize {
				return nil, fmt.Errorf("packet too large:%d", totalSize)
			} else if totalSize <= unpackSize {
				ss.r += sizeLen
				pkt := ss.buff[ss.r : ss.r+payload]
				ss.r += payload
				if ss.r == ss.w {
					ss.r = 0
					ss.w = 0
				}
				return pkt, nil
			} else {
				if totalSize > cap(ss.buff) {
					buff := make([]byte, totalSize)
					copy(buff, ss.buff[ss.r:ss.w])
					ss.buff = buff
				} else {
					//空间足够容纳下一个包，
					copy(ss.buff, ss.buff[ss.r:ss.w])
				}
				ss.w = ss.w - ss.r
				ss.r = 0
			}
		}

		var n int
		n, err = ss.read(readable, deadline)
		if n > 0 {
			ss.w += n
		}
		if nil != err {
			return
		}

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
			if msg, err := pb.Unmarshal("ss", uint32(cmd), data); err != nil {
				return nil, err
			} else {
				return NewMessage(to, from, msg), nil
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
