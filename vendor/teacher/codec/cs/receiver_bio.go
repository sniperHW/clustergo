// +build !aio

package cs

import (
	//"fmt"
	"github.com/sniperHW/kendynet"
	"net"
)

/*
 * 不设置 unpackMag 时，全部拆包
 * 设置时，遇到 unpackMag 注册的消息，拆包。其他消息拆分为字节消息
 */
func NewReceiver(namespace string, unpackMsg ...map[uint16]bool) *Receiver {
	receiver := &Receiver{}
	receiver.namespace = namespace
	receiver.buffer = make([]byte, MaxPacketSize*2)
	if len(unpackMsg) > 0 {
		receiver.unpackMsg = unpackMsg[0]
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
		msg, err = this.Unpack()

		if nil != msg {
			return msg, nil
		} else if err == nil {
			if this.w == this.r {
				this.w = 0
				this.r = 0
			} else if uint64(cap(this.buffer))-this.w < MaxPacketSize/4 {
				copy(this.buffer, this.buffer[this.r:this.w])
				this.w = this.w - this.r
				this.r = 0
			}

			conn := sess.GetUnderConn().(*net.TCPConn)
			n, err := conn.Read(this.buffer[this.w:])

			if n > 0 {
				this.w += uint64(n) //增加待解包数据
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
