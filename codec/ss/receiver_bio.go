// +build !aio

package ss

import (
	//"fmt"
	//"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	//"github.com/sniperHW/kendynet/rpc"
	"github.com/sniperHW/sanguo/cluster/addr"
	"net"
	//"github.com/sniperHW/sanguo/cluster/rpcerr"
	//"github.com/sniperHW/sanguo/codec/pb"
)

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

func (this *Receiver) Unpack() (msg interface{}, err error) {
	var packetSize uint64
	msg, packetSize, err = this.unpack(this.buffer, this.r, this.w)
	if nil != msg {
		this.r += packetSize
	}
	return
}

func (this *Receiver) ReceiveAndUnpack(sess kendynet.StreamSession) (interface{}, error) {
	var msg interface{}
	var err error
	for {
		this.unpackCount++
		msg, err = this.Unpack()

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

			//fmt.Println("recv", n)

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

func (this *Receiver) GetRecvBuff() []byte {
	return nil
}

func (this *Receiver) OnData([]byte) {

}

func (this *Receiver) OnSocketClose() {

}
