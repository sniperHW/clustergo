package codec

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/sanguo/codec/cs"
	"github.com/sniperHW/sanguo/codec/pb"
	"net"
)

type Receiver struct {
	buffer      []byte
	w           uint64
	r           uint64
	namespace   string
	gateMessage map[string]bool
}

func NewReceiver(namespace string, gateMessage map[string]bool) *Receiver {
	return &Receiver{
		namespace:   namespace,
		buffer:      make([]byte, cs.MaxPacketSize*2),
		gateMessage: gateMessage,
	}
}

func isCompress(flag uint16) bool {
	return flag&0x8000 > 0
}

func (this *Receiver) unPack() (interface{}, error) {
	unpackSize := uint64(this.w - this.r)
	if unpackSize >= 6 { //sizeLen + sizeFlag + sizeCmd

		var payload uint16
		var flag uint16
		var cmd uint16
		var err error
		var buff []byte
		var msg proto.Message

		reader := kendynet.NewReader(kendynet.NewByteBuffer(this.buffer[this.r:], unpackSize))

		if payload, err = reader.GetUint16(); err != nil {
			return nil, err
		}

		fullSize := uint64(payload) + cs.SizeLen

		if fullSize >= cs.MaxPacketSize {
			return nil, fmt.Errorf("packet too large %d", fullSize)
		}

		if uint64(payload) == 0 {
			return nil, fmt.Errorf("zero packet")
		}

		if fullSize <= unpackSize {
			if flag, err = reader.GetUint16(); err != nil {
				return nil, err
			}

			if cmd, err = reader.GetUint16(); err != nil {
				return nil, err
			}

			size := payload - (cs.SizeCmd + cs.SizeFlag)
			if buff, err = reader.GetBytes(uint64(size)); err != nil {
				return nil, err
			}

			msgName := pb.GetNameByID(this.namespace, uint32(cmd))

			if this.gateMessage[msgName] {
				if msg, err = pb.Unmarshal(this.namespace, uint32(cmd), buff); err != nil {
					return nil, err
				}

				this.r += fullSize

				message := &Message{
					name:   msgName,
					seriNO: (flag & 0x3FFF),
					data:   msg,
				}
				return message, nil
			} else {
				//透传消息
				message := NewRelayMessage(this.buffer[this.r : this.r+fullSize])
				this.r += fullSize
				return message, nil
			}

		} else {
			return nil, nil
		}
	}
	return nil, nil
}

func (this *Receiver) ReceiveAndUnpack(sess kendynet.StreamSession) (interface{}, error) {
	var msg interface{}
	var err error
	for {
		msg, err = this.unPack()

		if nil != msg {
			return msg, nil
		} else if err == nil {
			if this.w == this.r {
				this.w = 0
				this.r = 0
				b := this.buffer[this.w:]
				for k, _ := range b {
					b[k] = 0
				}
			} else if uint64(cap(this.buffer))-this.w < cs.MaxPacketSize/4 {
				copy(this.buffer, this.buffer[this.r:this.w])
				this.w = this.w - this.r
				this.r = 0
				b := this.buffer[this.w:]
				for k, _ := range b {
					b[k] = 0
				}
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
