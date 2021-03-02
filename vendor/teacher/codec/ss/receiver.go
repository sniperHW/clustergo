package ss

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/rpc"
	//"net"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/sanguo/cluster/rpcerr"
	"github.com/sniperHW/sanguo/codec/pb"
)

const (
	maxPacketSize uint64 = 65535 * 4
	minSize       uint64 = sizeLen + sizeFlag
)

type Receiver struct {
	buffer      []byte
	w           uint64
	r           uint64
	ns_msg      string
	ns_req      string
	ns_resp     string
	lastRecved  int
	recvCount   int
	unpackCount int
	selfAddr    addr.LogicAddr
}

func (this *Receiver) isTarget(to addr.LogicAddr) bool {
	return this.selfAddr == to
}

func (this *Receiver) unpack(buffer []byte, r uint64, w uint64) (ret interface{}, packetSize uint64, err error) {
	unpackSize := uint64(w - r)
	if unpackSize >= minSize {
		var totalSize uint64
		var payload uint32
		var flag byte
		var cmd uint16
		var buff []byte
		var errStr string
		var msg proto.Message
		var to addr.LogicAddr
		var from addr.LogicAddr
		var t uint32
		var f uint32
		var addrSize uint32
		reader := kendynet.NewReader(kendynet.NewByteBuffer(buffer[r:], unpackSize))
		if payload, err = reader.GetUint32(); err != nil {
			return
		}

		if uint64(payload) == 0 {
			err = fmt.Errorf("zero payload")
			return
		}

		totalSize = uint64(payload + sizeLen)

		if totalSize > maxPacketSize {
			err = fmt.Errorf("large packet %d", totalSize)
			return
		}

		packetSize = totalSize

		if totalSize <= unpackSize {

			if flag, err = reader.GetByte(); err != nil {
				return
			}

			if isRelay(flag) {

				if t, err = reader.GetUint32(); err != nil {
					return
				}
				if f, err = reader.GetUint32(); err != nil {
					return
				}
				to = addr.LogicAddr(t)
				from = addr.LogicAddr(f)
				addrSize = sizeTo + sizeFrom
			}

			if (isRelay(flag) && this.isTarget(to)) || !isRelay(flag) {

				if cmd, err = reader.GetUint16(); err != nil {
					return
				}

				tt := getMsgType(flag)
				if tt == MESSAGE {
					//普通消息
					size := payload - (sizeCmd + sizeFlag + addrSize)
					if buff, err = reader.GetBytes(uint64(size)); err != nil {
						return
					}
					if msg, err = pb.Unmarshal(this.ns_msg, uint32(cmd), buff); err != nil {
						return
					}
					ret = NewMessage(msg, to, from)
				} else {
					var seqNO uint64
					if seqNO, err = reader.GetUint64(); err != nil {
						return
					}

					size := payload - (sizeCmd + sizeFlag + sizeRPCSeqNo + addrSize)
					if tt == RPCERR {
						//RPC响应错误信息
						if errStr, err = reader.GetString(uint64(size)); err != nil {
							return
						}

						ret = NewMessage(&rpc.RPCResponse{Seq: seqNO, Err: rpcerr.GetErrorByShortStr(errStr)}, to, from)
					} else if tt == RPCRESP {
						//RPC响应
						if buff, err = reader.GetBytes(uint64(size)); err != nil {
							return
						}
						if msg, err = pb.Unmarshal(this.ns_resp, uint32(cmd), buff); err != nil {
							return
						}
						ret = NewMessage(&rpc.RPCResponse{Seq: seqNO, Ret: msg}, to, from)
					} else if tt == RPCREQ {
						//RPC请求
						if buff, err = reader.GetBytes(uint64(size)); err != nil {
							return
						}
						if msg, err = pb.Unmarshal(this.ns_req, uint32(cmd), buff); err != nil {
							return
						}
						ret = NewMessage(&rpc.RPCRequest{
							Seq:      seqNO,
							Method:   pb.GetNameByID(this.ns_req, uint32(cmd)),
							NeedResp: getNeedRPCResp(flag),
							Arg:      msg,
						}, to, from)
					} else {
						err = fmt.Errorf("invaild message type")
					}
				}
			} else {
				ret = NewRelayMessage(to, from, buffer[r:r+totalSize])
			}
		}
	}
	return
}
