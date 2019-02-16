package cs

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"sanguo/codec/pb"
	_ "sanguo/protocol/cs"

	"github.com/sniperHW/kendynet"
)

const (
	SizeLen  = 2
	SizeFlag = 2
	SizeCmd  = 2
)

/*
*  flag
*  低14位表示seriNO
*  高1位表示是否压缩
*  高2位表示是否加密
 */

type Encoder struct {
	namespace string
	zipBuff   bytes.Buffer
	zipWriter *zlib.Writer
}

func NewEncoder(namespace string) *Encoder {
	return &Encoder{namespace: namespace}
}

func setCompressFlag(flag uint16) uint16 {
	return flag | 0x8000
}

func (this *Encoder) EnCode(o interface{}) (kendynet.Message, error) {
	switch o.(type) {
	case *Message:
		var pbbytes []byte
		var cmd uint32
		var err error

		msg := o.(*Message)

		flag := uint16(msg.GetSeriNo())

		if pbbytes, cmd, err = pb.Marshal(this.namespace, msg.GetData()); err != nil {
			return nil, err
		}

		if msg.IsCompress() {
			flag = setCompressFlag(flag)
			//对pb数据执行压缩
			if nil == this.zipWriter {
				this.zipWriter = zlib.NewWriter(&this.zipBuff)
			} else {
				this.zipBuff.Reset()
				this.zipWriter.Reset(&this.zipBuff)
			}

			this.zipWriter.Write(pbbytes)
			this.zipWriter.Flush()
			pbbytes = this.zipBuff.Bytes()
		}

		totalLen := len(pbbytes) + SizeLen + SizeFlag + SizeCmd
		//fmt.Println("------------------>", totalLen)

		if uint64(totalLen) > MaxPacketSize {
			return nil, fmt.Errorf("packet too large totalLen:%d", totalLen)
		}

		//len + flag + cmd + pbbytes
		buff := kendynet.NewByteBuffer(totalLen)
		//写payload大小
		buff.AppendUint16(uint16(totalLen - SizeLen))
		//写flag
		buff.AppendUint16(flag)
		//写cmd
		buff.AppendUint16(uint16(cmd))
		//写数据
		buff.AppendBytes(pbbytes)
		return buff, nil
		break
	default:
		break
	}
	return nil, fmt.Errorf("invaild object type")
}
