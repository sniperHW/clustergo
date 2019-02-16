package ss

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/rpc"
	"net"
	"sanguo/cluster/addr"
	"sanguo/codec/pb"
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

func NewReceiver(ns_msg, ns_req, ns_resp string, selfAddr ...addr.LogicAddr) *Receiver {
	receiver := &Receiver{}
	receiver.ns_msg = ns_msg
	receiver.ns_req = ns_req
	receiver.ns_resp = ns_resp
	receiver.buffer = make([]byte, maxPacketSize*2)

	if len(selfAddr) > 0 {
		receiver.selfAddr = selfAddr[0]
	}

	return receiver
}

func (this *Receiver) isTarget(to addr.LogicAddr) bool {
	return this.selfAddr == to
}

func (this *Receiver) unPack() (interface{}, error) {
	unpackSize := uint64(this.w - this.r)
	if unpackSize >= minSize {
		var totalSize uint64
		var payload uint32
		var flag byte
		var cmd uint16
		var err error
		var buff []byte
		var errStr string
		var msg proto.Message
		var to addr.LogicAddr
		var from addr.LogicAddr
		var t uint32
		var f uint32
		var addrSize uint32
		reader := kendynet.NewReader(kendynet.NewByteBuffer(this.buffer[this.r:], unpackSize))
		if payload, err = reader.GetUint32(); err != nil {
			return nil, err
		}

		if uint64(payload) == 0 {
			kendynet.Infoln(this.lastRecved, this.r, this.w, this.buffer[this.r:this.w])
			return nil, fmt.Errorf("zero payload")
		}

		totalSize = uint64(payload + sizeLen)

		if totalSize > maxPacketSize {
			kendynet.Infoln(this.recvCount, this.unpackCount, this.lastRecved, this.r, this.w, this.buffer[this.r:this.w])
			return nil, fmt.Errorf("large packet %d", totalSize)
		}

		if totalSize <= unpackSize {

			if flag, err = reader.GetByte(); err != nil {
				return nil, err
			}

			if isRelay(flag) {

				//fmt.Println("isRelay")

				//kendynet.Debugln("isRelay")

				if t, err = reader.GetUint32(); err != nil {
					return nil, err
				}
				if f, err = reader.GetUint32(); err != nil {
					return nil, err
				}
				to = addr.LogicAddr(t)
				from = addr.LogicAddr(f)
				addrSize = sizeTo + sizeFrom
				//kendynet.Debugln(to.String(), from.String(), this.selfAddr.String())
			}

			if (isRelay(flag) && this.isTarget(to)) || !isRelay(flag) {

				if cmd, err = reader.GetUint16(); err != nil {
					return nil, err
				}

				tt := getMsgType(flag)
				if tt == MESSAGE {
					//普通消息
					size := payload - (sizeCmd + sizeFlag + addrSize)
					if buff, err = reader.GetBytes(uint64(size)); err != nil {
						return nil, err
					}
					if msg, err = pb.Unmarshal(this.ns_msg, uint32(cmd), buff); err != nil {
						return nil, err
					}
					this.r += totalSize
					//kendynet.Debugln(to.String(), from.String(), this.selfAddr.String())
					return NewMessage(pb.GetNameByID(this.ns_msg, uint32(cmd)), msg, to, from), nil
				} else {
					var seqNO uint64
					if seqNO, err = reader.GetUint64(); err != nil {
						return nil, err
					}

					size := payload - (sizeCmd + sizeFlag + sizeRPCSeqNo + addrSize)
					if tt == RPCERR {
						//RPC响应错误信息
						if errStr, err = reader.GetString(uint64(size)); err != nil {
							return nil, err
						}
						this.r += totalSize
						return NewMessage("rpc", &rpc.RPCResponse{Seq: seqNO, Err: fmt.Errorf(errStr)}, to, from), nil
					} else if tt == RPCRESP {
						//RPC响应
						if buff, err = reader.GetBytes(uint64(size)); err != nil {
							return nil, err
						}
						if msg, err = pb.Unmarshal(this.ns_resp, uint32(cmd), buff); err != nil {
							return nil, err
						}
						this.r += totalSize
						return NewMessage("rpc", &rpc.RPCResponse{Seq: seqNO, Ret: msg}, to, from), nil
					} else if tt == RPCREQ {
						//RPC请求
						if buff, err = reader.GetBytes(uint64(size)); err != nil {
							return nil, err
						}
						if msg, err = pb.Unmarshal(this.ns_req, uint32(cmd), buff); err != nil {
							return nil, err
						}
						this.r += totalSize
						return NewMessage("rpc", &rpc.RPCRequest{
							Seq:      seqNO,
							Method:   pb.GetNameByID(this.ns_req, uint32(cmd)),
							NeedResp: getNeedRPCResp(flag),
							Arg:      msg,
						}, to, from), nil
					} else {
						return nil, fmt.Errorf("invaild message type")

					}
				}
			} else {
				msg := NewRelayMessage(to, from, this.buffer[this.r:this.r+totalSize])
				this.r += totalSize
				return msg, nil
			}
		}
	}
	return nil, nil
}

func (this *Receiver) ReceiveAndUnpack(sess kendynet.StreamSession) (interface{}, error) {
	var msg interface{}
	var err error
	for {
		this.unpackCount++
		msg, err = this.unPack()

		if nil != msg {
			return msg, nil
		} else if err == nil {
			if this.w == this.r {
				this.w = 0
				this.r = 0
			} else if uint64(cap(this.buffer))-this.w < maxPacketSize/4 {
				copy(this.buffer, this.buffer[this.r:this.w])
				this.w = this.w - this.r
				this.r = 0
			}

			conn := sess.GetUnderConn().(*net.TCPConn)
			n, err := conn.Read(this.buffer[this.w:])
			this.recvCount++

			if n > 0 {
				this.w += uint64(n) //增加待解包数据
				this.lastRecved = n
			}
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
}

func (this *Receiver) TestUnpack(buff []byte) (interface{}, error) {
	copy(this.buffer, buff[:])
	this.r = 0
	this.w = uint64(len(buff))
	packet, err := this.unPack()
	return packet, err
}
