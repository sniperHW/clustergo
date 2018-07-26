package ss

import (
	"github.com/sniperHW/kendynet"
	//"github.com/sniperHW/kendynet/socket/stream_socket"
	"sanguo/codec/pb"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet/rpc"
	"fmt"
	"os"
	"time"
	//"sync"
	"net"
)

const (
	maxPacketSize uint64 = 65535
)

type Receiver struct {
	buffer         []byte
	w              uint64
	r              uint64
	ns_msg  	   string
	ns_req         string
	ns_resp        string
}

func NewReceiver(ns_msg,ns_req,ns_resp string) (*Receiver) {
	receiver := &Receiver{}
	receiver.ns_msg   = ns_msg
	receiver.ns_req   = ns_req
	receiver.ns_resp  = ns_resp
	receiver.buffer   = make([]byte,maxPacketSize*2)
	return receiver
}

func (this *Receiver) unPack() (ret interface{},err error) {
	unpackSize := uint64(this.w - this.r)
	if unpackSize >= 5 { //sizeLen + sizeFlag + sizeCmd
		var payload uint16
		var flag byte
		var cmd  uint16
		var err  error
		var buff []byte
		var errStr string
		var msg proto.Message
		var totalSize uint64

		for {

			reader := kendynet.NewReader(kendynet.NewByteBuffer(this.buffer[this.r:],unpackSize))
			if payload,err = reader.GetUint16(); err != nil {
				break
			}

			if uint64(payload) == 0 {
				err = fmt.Errorf("zero payload")
				break
			}

			if uint64(payload) + sizeLen > maxPacketSize {
				err = fmt.Errorf("large packet %d",uint64(payload) + sizeLen)
				break
			}

			totalSize = uint64(payload + sizeLen)


			if totalSize <= unpackSize {

				if flag,err = reader.GetByte(); err != nil {
					break
				}	
				
				if cmd,err = reader.GetUint16(); err != nil {
					break
				}
				
				tt := getMsgType(flag)
				if tt == MESSAGE {
					//普通消息
					size := payload - (sizeCmd + sizeFlag)
					if buff,err = reader.GetBytes(uint64(size)); err != nil {
						break
					}
					if msg,err = pb.Unmarshal(this.ns_msg,uint32(cmd),buff); err != nil {
						break
					}
					ret = NewMessage(pb.GetNameByID(this.ns_msg,uint32(cmd)),msg)				
				} else {
					var seqNO uint64
					if seqNO,err = reader.GetUint64(); err != nil {
						break
					}

					size := payload - (sizeCmd + sizeFlag + sizeRPCSeqNo)
					if tt == RPCERR {
						//RPC响应错误信息
						if errStr,err = reader.GetString(uint64(size)); err != nil {
							break
						}
						ret = &rpc.RPCResponse{Seq:seqNO,Err:fmt.Errorf(errStr)}
						break
					} else if tt == RPCRESP {
						//RPC响应
						if buff,err = reader.GetBytes(uint64(size)); err != nil {
							break
						}
						if msg,err = pb.Unmarshal(this.ns_resp,uint32(cmd),buff); err != nil {
							break
						}
						ret = &rpc.RPCResponse{Seq:seqNO,Ret:msg}
					} else if tt == RPCREQ {
						//RPC请求
						if buff,err = reader.GetBytes(uint64(size)); err != nil {
							break
						}
						if msg,err = pb.Unmarshal(this.ns_req,uint32(cmd),buff); err != nil {
							break
						}
						req := &rpc.RPCRequest{}
						req.Seq = seqNO
						req.Method = pb.GetNameByID(this.ns_req,uint32(cmd))
						req.NeedResp = getNeedRPCResp(flag)
						req.Arg = msg	
						ret = req
					} else {
						kendynet.Infof("invaild message type\n")
						os.Exit(0)
						err = fmt.Errorf("invaild message type:%d\n",tt)
					}
				}
			}
			break
		}
		if err == nil && totalSize > 0 {
			this.r += totalSize
		}
	}
	return
}

func (this *Receiver) check(buff []byte, n int) {
	l := len(buff)
	l = l / 4
	for i:=0;i < l;i++{
		var j int
		for j=0;j<4;j++{
			if buff[i*4+j] != 0xFF {
				break
			}
		}
		if j == 4 {
			kendynet.Infoln(n,buff)
			time.Sleep(time.Second)
			os.Exit(0)
		}
	}
}


func (this *Receiver) ReceiveAndUnpack(sess kendynet.StreamSession) (interface{},error) {
	var msg interface{}
	var err error
	for {
		msg,err = this.unPack()
		if nil != msg {
			return msg,nil
		} else if err == nil {

			if uint64(cap(this.buffer)) - this.w < maxPacketSize / 4 {	
				if this.w != this.r {
					copy(this.buffer,this.buffer[this.r:this.w])
				}
				this.w = this.w - this.r
				this.r = 0
			}

			conn := sess.GetUnderConn().(*net.TCPConn)
			//n,err := conn.Read(this.buffer[this.w:])

			buff  := [4096]byte{}
			for k,_ := range(buff) {
				buff[k]=0xFF
			}

			n,err := conn.Read(buff[:])

			if n > 0 {
				this.check(buff[:n],n)	
				copy(this.buffer[this.w:],buff[:n])
				this.w += uint64(n) //增加待解包数据		
			}
			if err != nil {
				return nil,err
			}
		} else {
			return nil,err
		}
	}
}


func (this *Receiver) TestUnpack(buff []byte) (interface{},error) {
	copy(this.buffer,buff[:])
	this.r = 0
	this.w = uint64(len(buff))
	packet,err := this.unPack()
	return packet,err
}
