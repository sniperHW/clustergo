package ss

import(
	"github.com/golang/protobuf/proto"
)

type Message struct {
	name      string
	data      proto.Message
}

func NewMessage(name string,data proto.Message) *Message {
	return &Message{name:name,data:data}
}

func (this *Message) GetData() proto.Message {
	return this.data
}

func (this *Message) GetName() string {
	return this.name
}
