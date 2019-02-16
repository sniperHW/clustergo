package codec

import (
	"fmt"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/sanguo/codec/cs"
	"github.com/sniperHW/sanguo/codec/pb"
	_ "github.com/sniperHW/sanguo/protocol/cs"
	"reflect"
)

type Encoder struct {
	namespace string
}

func NewEncoder(namespace string) *Encoder {
	return &Encoder{namespace: namespace}
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

		totalLen := len(pbbytes) + cs.SizeLen + cs.SizeFlag + cs.SizeCmd

		if uint64(totalLen) > cs.MaxPacketSize {
			return nil, fmt.Errorf("packet too large totalLen:%d", totalLen)
		}

		//len + flag + cmd + pbbytes
		buff := kendynet.NewByteBuffer(totalLen)
		//写payload大小
		buff.AppendUint16(uint16(totalLen - cs.SizeLen))
		//写flag
		buff.AppendUint16(flag)
		//写cmd
		buff.AppendUint16(uint16(cmd))
		//写数据
		buff.AppendBytes(pbbytes)
		return buff, nil
		break
	case []byte:
		//透传消息
		bytes := o.([]byte)
		return kendynet.NewByteBuffer(bytes, len(bytes)), nil
		break
	default:
		break
	}
	return nil, fmt.Errorf("invaild object type:%s", reflect.TypeOf(o).String())
}
