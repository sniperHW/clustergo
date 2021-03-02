package cs

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/sanguo/codec/pb"
	"io"
	"io/ioutil"
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

func isCompress(flag uint16) bool {
	return flag&0x8000 > 0
}

func (this *Receiver) DirectUnpack(buff []byte) (interface{}, error) {
	if uint64(len(buff)) > MaxPacketSize {
		return nil, fmt.Errorf("packet too large totalLen:%d", len(buff))
	}
	msg, _, err := this.unpack(buff, 0, uint64(len(buff)))
	return msg, err
}

func (this *Receiver) ProtoMarshal(msg proto.Message) ([]byte, uint32, error) {
	return pb.Marshal(this.namespace, msg)
}

func (this *Receiver) ProtoUnmarshal(cmd uint16, data []byte) (proto.Message, error) {
	return pb.Unmarshal(this.namespace, uint32(cmd), data)
}

func (this *Receiver) unpack(buffer []byte, r uint64, w uint64) (ret interface{}, packetSize uint64, err error) {
	unpackSize := uint64(w - r)
	if unpackSize >= HeadSize {

		var payload uint16
		var seqNo uint32
		var flag uint16
		var cmd uint16
		var errCode uint16
		var buff []byte
		var msg proto.Message

		reader := kendynet.NewReader(kendynet.NewByteBuffer(buffer[r:], unpackSize))

		if payload, err = reader.GetUint16(); err != nil {
			return
		}

		fullSize := uint64(payload) + SizeLen

		if fullSize >= MaxPacketSize {
			err = fmt.Errorf("packet too large %d", fullSize)
			return
		}

		if uint64(payload) == 0 {
			err = fmt.Errorf("zero packet")
			return
		}

		packetSize = fullSize

		if fullSize <= unpackSize {
			if seqNo, err = reader.GetUint32(); err != nil {
				return
			}

			if flag, err = reader.GetUint16(); err != nil {
				return
			}

			if cmd, err = reader.GetUint16(); err != nil {
				return
			}

			if errCode, err = reader.GetUint16(); err != nil {
				return
			}

			size := payload - (HeadSize - SizeLen)
			if buff, err = reader.GetBytes(uint64(size)); err != nil {
				return
			}

			if this.unpackMsg != nil {
				if _, ok := this.unpackMsg[cmd]; !ok {
					//透传消息
					message := NewBytesMassage(this.buffer[r : r+fullSize])
					ret = message
					return
				}
			}

			if isCompress(flag) {
				this.zipBuff.Reset()
				this.zipBuff.Write(buff)
				var r io.ReadCloser
				r, err = zlib.NewReader(&this.zipBuff)
				if err != nil {
					return
				}

				buff, err = ioutil.ReadAll(r)
				r.Close()
				if err != nil {
					if err != io.ErrUnexpectedEOF && err != io.EOF {
						return
					}
				}
			}

			if errCode == 0 {
				if msg, err = pb.Unmarshal(this.namespace, uint32(cmd), buff); err != nil {
					return
				}
			}

			ret = &Message{
				seriNO:  seqNo,
				data:    msg,
				cmd:     cmd,
				errCode: errCode,
			}
			return
		} else {
			return
		}
	}
	return
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
