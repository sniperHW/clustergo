package ss

import (
	"encoding/binary"
	"fmt"

	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/codec/buffer"
	"github.com/sniperHW/clustergo/codec/pb"
	"github.com/sniperHW/clustergo/rpc"
	"github.com/sniperHW/clustergo/socket"
	"github.com/sniperHW/netgo/poolbuff"
	"google.golang.org/protobuf/proto"
)

const Namespace string = "ss"

var _ socket.Codec = (*SSCodec)(nil)

type SSCodec struct {
	selfAddr addr.LogicAddr
	rpcCodec rpc.Codec
	reader   buffer.BufferReader
	pbMeta   *pb.PbMeta
}

func NewCodec(selfAddr addr.LogicAddr, rpcCodec rpc.Codec) *SSCodec {
	return &SSCodec{
		selfAddr: selfAddr,
		rpcCodec: rpcCodec,
		pbMeta:   pb.GetMeta(Namespace),
		reader:   buffer.NewReader(binary.BigEndian, nil),
	}
}

// appendSSHeader writes the length-prefix placeholder + flag + to + from and returns
// the new dst plus the mark (offset of the length prefix) for a later finalizeFrame.
func appendSSHeader(dst []byte, m *Message, flag byte) ([]byte, int) {
	mark := len(dst)
	dst = append(dst, 0, 0, 0, 0) // length prefix placeholder
	dst = append(dst, flag)
	w := buffer.NeWriter(binary.BigEndian)
	dst = w.AppendUint32(dst, uint32(m.To()))
	dst = w.AppendUint32(dst, uint32(m.From()))
	return dst, mark
}

// finalizeFrame patches the 4-byte length prefix at mark and enforces MaxPacketSize.
// On overflow it rolls dst back to mark and returns 0 (encode nothing).
func finalizeFrame(dst []byte, mark int) ([]byte, int) {
	totalLen := len(dst) - mark
	if totalLen > socket.MaxPacketSize {
		return dst[:mark], 0
	}
	binary.BigEndian.PutUint32(dst[mark:], uint32(totalLen-sizeLen))
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
			dst, mark := appendSSHeader(dst, o, flag)
			w := buffer.NeWriter(binary.BigEndian)
			dst = w.AppendUint16(dst, o.cmd)
			dst = append(dst, msg...)
			return finalizeFrame(dst, mark)
		case proto.Message:
			setMsgType(&flag, PbMsg)
			dst, mark := appendSSHeader(dst, o, flag)
			w := buffer.NeWriter(binary.BigEndian)
			cmdMark := len(dst)
			dst = w.AppendUint16(dst, 0) // cmd placeholder, patched once Marshal returns the id
			var mErr error
			var cmd uint32
			dst, cmd, mErr = ss.pbMeta.Marshal(dst, msg)
			if mErr != nil {
				return dst[:mark], 0
			}
			binary.BigEndian.PutUint16(dst[cmdMark:], uint16(cmd))
			return finalizeFrame(dst, mark)
		case *rpc.RequestMsg:
			setMsgType(&flag, RpcReq)
			dst, mark := appendSSHeader(dst, o, flag)
			var e error
			dst, e = rpc.EncodeRequest(dst, msg, ss.rpcCodec)
			if e != nil {
				return dst[:mark], 0
			}
			return finalizeFrame(dst, mark)
		case *rpc.ResponseMsg:
			setMsgType(&flag, RpcResp)
			dst, mark := appendSSHeader(dst, o, flag)
			var e error
			dst, e = rpc.EncodeResponse(dst, msg, ss.rpcCodec)
			if e != nil {
				return dst[:mark], 0
			}
			return finalizeFrame(dst, mark)
		}
		return dst, 0
	case *RelayMessage:
		dst = append(dst, o.payload...)
		poolbuff.Put(o.payload)
		return dst, len(o.payload)
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
			if req, err := rpc.DecodeRequest(ss.reader.GetAll(), ss.rpcCodec); err != nil {
				return nil, err
			} else {
				return NewMessage(to, from, req), nil
			}
		case RpcResp:
			if resp, err := rpc.DecodeResponse(ss.reader.GetAll(), ss.rpcCodec); err != nil {
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
