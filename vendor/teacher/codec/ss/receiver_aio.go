// +build aio

package ss

import (
	//"fmt"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/sanguo/codec"
)

var bufferpool *BufferPool = codec.NewBufferPool(128 * 1024)

func GetBufferPool() *codec.BufferPool {
	return bufferpool
}

func NewReceiver(ns_msg, ns_req, ns_resp string, selfAddr ...addr.LogicAddr) *Receiver {
	receiver := &Receiver{}
	receiver.ns_msg = ns_msg
	receiver.ns_req = ns_req
	receiver.ns_resp = ns_resp

	if len(selfAddr) > 0 {
		receiver.selfAddr = selfAddr[0]
	}

	return receiver
}

func (this *Receiver) Unpack() (msg interface{}, err error) {
	if this.r != this.w {
		var packetSize uint64
		msg, packetSize, err = this.unpack(this.buffer, this.r, this.w)
		if nil != msg {

			this.r += packetSize
			if this.r == this.w {
				this.r = 0
				this.w = 0
				bufferpool.Release(this.buffer)
				this.buffer = nil
			}

		} else if nil == err {
			//新开一个buffer把未接完整的包先接收掉,处理完这个包之后再次启用sharebuffer模式
			buff := make([]byte, packetSize)
			copy(buff, this.buffer[this.r:this.w])
			this.w = this.w - this.r
			this.r = 0
			bufferpool.Release(this.buffer)
			this.buffer = buff
		}
	}
	return
}

func (this *Receiver) GetRecvBuff() []byte {
	if len(this.buffer) == 0 {
		//sharebuffer模式
		return nil
	} else {
		//之前有包没接收完，先把这个包接收掉
		return this.buffer[this.w:]
	}
}

func (this *Receiver) OnData(data []byte) {
	if len(this.buffer) == 0 {
		this.buffer = data
	}
	this.w += uint64(len(data))
}

func (this *Receiver) OnSocketClose() {
	bufferpool.Release(this.buffer)
}

func (this *Receiver) ReceiveAndUnpack(sess kendynet.StreamSession) (interface{}, error) {
	panic("should not go here")
	return nil, nil
}
