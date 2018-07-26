package cs

import (
	"github.com/sniperHW/kendynet"
	//"github.com/sniperHW/kendynet/socket/stream_socket"
	"sanguo/codec/pb"
	"github.com/golang/protobuf/proto"
	"fmt"
	"net"
	"time"
	"os"
)

const (
	maxPacketSize uint64 = 65535
)

type Receiver struct {
	buffer         []byte
	w              uint64
	r              uint64
	namespace      string
}

func NewReceiver(namespace string) (*Receiver) {
	receiver := &Receiver{}
	receiver.namespace = namespace
	receiver.buffer = make([]byte,maxPacketSize*2)
	return receiver
}

func (this *Receiver) unPack() (interface{},error) {
	unpackSize := uint64(this.w - this.r)
	if unpackSize >= 6 {//sizeLen + sizeFlag + sizeCmd
		
		var payload uint16
		var flag uint16
		var cmd  uint16
		var err error
		var buff []byte
		var msg proto.Message

		reader := kendynet.NewReader(kendynet.NewByteBuffer(this.buffer[this.r:],unpackSize))


		if payload,err = reader.GetUint16(); err != nil {
			return nil,err
		}


		fullSize := uint64(payload) + sizeLen

		if fullSize >= maxPacketSize {
			return nil,fmt.Errorf("packet too large %d",fullSize)
		}

		if uint64(payload) == 0 {
			return nil,fmt.Errorf("zero packet")
		}

		if fullSize <= unpackSize {
			if flag,err = reader.GetUint16(); err != nil {
				return nil,err
			}

			if cmd,err = reader.GetUint16(); err != nil {
				return nil,err
			}
			
			size := payload - (sizeCmd + sizeFlag)
			if buff,err = reader.GetBytes(uint64(size)); err != nil {
				return nil,err
			}
			
			if msg,err = pb.Unmarshal(this.namespace,uint32(cmd),buff); err != nil {
				return nil,err
			}

			this.r += fullSize

			message := &Message{
				name : pb.GetNameByID(this.namespace,uint32(cmd)),
				seriNO : (flag & 0x3FFF),
				data : msg,
			}
			return message,nil
		} else {
			return nil,nil
		}
	}
	return nil,nil
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
	this.w = 0
	this.r = uint64(len(buff))
	return this.unPack()
}