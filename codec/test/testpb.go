package main

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/sanguo/codec/pb"
	"github.com/sniperHW/sanguo/codec/test/testproto"
)

func main() {
	if err := pb.Register("test", &testproto.Test{}, 1); nil != err {
		fmt.Println(err)
	}

	t := &testproto.Test{}
	t.B = proto.Int32(1)
	t.A = proto.String("hello")

	data, id, err := pb.Marshal("test", t)

	if nil != err {
		fmt.Println(err)
	}

	tmp := make([]byte, len(data))
	copy(tmp, data)
	tmp = append(tmp, 0xFF)
	tmp = append(tmp, 0xFF)

	tt, err := pb.Unmarshal("test", id, tmp)

	if nil != err {
		fmt.Println(err)
	}

	fmt.Println(tt.(*testproto.Test).GetA())
	fmt.Println(tt.(*testproto.Test).GetB())

	/*	s := make([]int,0)
		s = append(s,1)
		s = append(s,2)

		fmt.Println(len(s),cap(s))
		s = s[0:0]
		fmt.Println(len(s),cap(s))

		v := make(struct{a int;b int},0)

		v = append(v,struct{a:1,b:2})

		fmt.Println(v)
	*/
}
