package main

import (
	"sanguo/codec/pb"
	"sanguo/codec/cs"
	"sanguo/codec/test/testproto"
	"github.com/golang/protobuf/proto"
	"fmt"
)

func main() {
	if err := pb.Register("cs",&testproto.Test{},1); nil != err {
		fmt.Println(err)
	}

	t  := &testproto.Test{}
	t.B = proto.Int32(1)
	t.A = proto.String("hello")

	msg := cs.NewMessage(0,t)

	encoder := cs.NewEncoder("cs")

	buff,err := encoder.EnCode(msg)

	if nil != err {
		fmt.Println(err)
	}

	if nil != buff {

		receiver := cs.NewReceiver("cs")

		r,err := receiver.TestUnpack(buff.Bytes())

		if nil != err {
			fmt.Println(err)
		}

		msg = r.(cs.Message)
		fmt.Println(msg.GetSeriNo())
		fmt.Println(msg.GetData().(*testproto.Test).GetA())
	}
}