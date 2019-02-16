package cs

import (
	"github.com/golang/protobuf/proto"
	"reflect"
)

type Message struct {
	seriNO   uint16
	data     proto.Message
	name     string
	compress bool //是否压缩
}

func NewMessage(seriNO uint16, data proto.Message) *Message {
	return &Message{seriNO: seriNO & 0x3FFF, data: data}
}

func (this *Message) SetCompress() *Message {
	this.compress = true
	return this
}

func (this *Message) IsCompress() bool {
	return this.compress
}

func (this *Message) GetData() proto.Message {
	return this.data
}

func (this *Message) GetSeriNo() uint16 {
	return this.seriNO
}

func (this *Message) GetName() string {
	if "" == this.name {
		return reflect.TypeOf(this.data).String()
	} else {
		return this.name
	}
}
