package cs

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"io/ioutil"
	"net"

	//"os"
	"github.com/sniperHW/sanguo/codec/pb"
	//"time"

	"encoding/binary"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
)

const (
	MaxPacketSize uint64 = 65535
)

type Receiver struct {
	buffer    []byte
	w         uint64
	r         uint64
	namespace string
	zipBuff   bytes.Buffer
	unpackMsg map[uint16]bool
}

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

func isCompress(flag uint16) bool {
	return flag&0x8000 > 0
}

func (this *Receiver) unPack() (interface{}, error) {
	unpackSize := uint64(this.w - this.r)
	if unpackSize >= HeadSize {

		var payload uint16
		var seqNo uint32
		var flag uint16
		var cmd uint16
		var errCode uint16
		var err error
		var buff []byte
		var msg proto.Message

		reader := kendynet.NewReader(kendynet.NewByteBuffer(this.buffer[this.r:], unpackSize))

		if payload, err = reader.GetUint16(); err != nil {
			return nil, err
		}

		fullSize := uint64(payload) + SizeLen

		if fullSize >= MaxPacketSize {
			return nil, fmt.Errorf("packet too large %d", fullSize)
		}

		if uint64(payload) == 0 {
			return nil, fmt.Errorf("zero packet")
		}

		if fullSize <= unpackSize {
			if seqNo, err = reader.GetUint32(); err != nil {
				return nil, err
			}

			if flag, err = reader.GetUint16(); err != nil {
				return nil, err
			}

			if cmd, err = reader.GetUint16(); err != nil {
				return nil, err
			}

			if errCode, err = reader.GetUint16(); err != nil {
				return nil, err
			}

			size := payload - (HeadSize - SizeLen)
			if buff, err = reader.GetBytes(uint64(size)); err != nil {
				return nil, err
			}

			if this.unpackMsg != nil {
				if _, ok := this.unpackMsg[cmd]; !ok {
					//透传消息
					message := NewBytesMassage(this.buffer[this.r : this.r+fullSize])
					this.r += fullSize
					return message, nil
				}
			}

			if isCompress(flag) {
				this.zipBuff.Reset()
				this.zipBuff.Write(buff)
				var r io.ReadCloser
				r, err = zlib.NewReader(&this.zipBuff)
				if err != nil {
					return nil, err
				}

				buff, err = ioutil.ReadAll(r)
				r.Close()
				if err != nil {
					if err != io.ErrUnexpectedEOF && err != io.EOF {
						return nil, err
					}
				}
			}

			if errCode == 0 {
				if msg, err = pb.Unmarshal(this.namespace, uint32(cmd), buff); err != nil {
					return nil, err
				}
			}

			this.r += fullSize

			message := &Message{
				//name:    pb.GetNameByID(this.namespace, uint32(cmd)),
				seriNO:  seqNo,
				data:    msg,
				cmd:     cmd,
				errCode: errCode,
			}
			return message, nil
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

func (this *Receiver) DirectUnpack(buff []byte) (interface{}, error) {
	if uint64(len(buff)) > MaxPacketSize {
		return nil, fmt.Errorf("packet too large totalLen:%d", len(buff))
	}
	copy(this.buffer, buff[:])
	this.w = uint64(len(buff))
	this.r = 0
	return this.unPack()
}

//SizeLen  = 2
//SizeSeqNo  = 4
//SizeFlag = 2
//SizeCmd  = 2
//SizeErr  = 2
func FetchSeqCmdCode(buff []byte) (uint32, uint16, uint16) {
	seqno := binary.BigEndian.Uint32(buff[2 : 2+4])
	cmd := binary.BigEndian.Uint16(buff[8 : 8+2])
	code := binary.BigEndian.Uint16(buff[10 : 10+2])
	return seqno, cmd, code
}
