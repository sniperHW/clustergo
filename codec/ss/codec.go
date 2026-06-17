package ss

import (
	"encoding/binary"
	"fmt"

	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/codec/buffer"
	"github.com/sniperHW/clustergo/codec/pb"
	"github.com/sniperHW/clustergo/rpc"
	"github.com/sniperHW/clustergo/socket"
	"google.golang.org/protobuf/proto"
)

const Namespace string = "ss"

var _ socket.Codec = (*SSCodec)(nil)

type SSCodec struct {
	selfAddr addr.LogicAddr
	reader   buffer.BufferReader
	pbMeta   *pb.PbMeta
}

func NewCodec(selfAddr addr.LogicAddr) *SSCodec {
	return &SSCodec{
		selfAddr: selfAddr,
		pbMeta:   pb.GetMeta(Namespace),
		reader:   buffer.NewReader(binary.BigEndian, nil),
	}
}

func (ss *SSCodec) encode(dst []byte, m *Message, cmd uint16, flag byte, data []byte) ([]byte, int) {
	payloadLen := sizeFlag + sizeToAndFrom + len(data)
	if flag == PbMsg || flag == BinMsg {
		payloadLen += sizeCmd
	}
	totalLen := sizeLen + payloadLen
	if totalLen > socket.MaxPacketSize {
		return dst, 0
	}
	w := buffer.NeWriter(binary.BigEndian)
	mark := len(dst)
	dst = append(dst, 0, 0, 0, 0) // length prefix placeholder
	binary.BigEndian.PutUint32(dst[mark:], uint32(payloadLen))
	dst = append(dst, flag)
	dst = w.AppendUint32(dst, uint32(m.To()))
	dst = w.AppendUint32(dst, uint32(m.From()))
	if flag == PbMsg || flag == BinMsg {
		dst = w.AppendUint16(dst, cmd)
	}
	dst = append(dst, data...)
	return dst, totalLen
}

func (ss *SSCodec) Encode(dst []byte, o interface{}) ([]byte, int) {
	switch o := o.(type) {
	case *Message:
		flag := byte(0)
		switch msg := o.Payload().(type) {
		case []byte:
			if len(msg) == 0 {
				return dst, 0
			}
			setMsgType(&flag, BinMsg)
			return ss.encode(dst, o, o.cmd, flag, msg)
		case proto.Message:
			data, cmd, err := ss.pbMeta.Marshal(msg)
			if err != nil {
				return dst, 0
			}
			setMsgType(&flag, PbMsg)
			return ss.encode(dst, o, uint16(cmd), flag, data)
		case *rpc.RequestMsg:
			setMsgType(&flag, RpcReq)
			return ss.encode(dst, o, 0, flag, rpc.EncodeRequest(msg))
		case *rpc.ResponseMsg:
			setMsgType(&flag, RpcResp)
			return ss.encode(dst, o, 0, flag, rpc.EncodeResponse(msg))
		}
		return dst, 0
	case *RelayMessage:
		return append(dst, o.Payload()...), len(o.Payload())
	default:
		return dst, 0
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
			if req, err := rpc.DecodeRequest(ss.reader.GetAll()); err != nil {
				return nil, err
			} else {
				return NewMessage(to, from, req), nil
			}
		case RpcResp:
			if resp, err := rpc.DecodeResponse(ss.reader.GetAll()); err != nil {
				return nil, err
			} else {
				return NewMessage(to, from, resp), nil
			}
		default:
			return nil, fmt.Errorf("invaild packet type")
		}
	} else {
		return NewRelayMessage(to, from, payload), nil
	}
}
