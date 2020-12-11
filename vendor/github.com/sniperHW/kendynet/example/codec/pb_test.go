package codec

import (
	//"fmt"
	"bytes"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet/example/pb"
	"github.com/sniperHW/kendynet/example/testproto"
	"log"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"testing"
)

func (this *PBReceiver) append(data []byte) {
	n := len(data)
	copy(this.recvBuff, data)
	this.recvBuff = this.recvBuff[n:]
	this.unpackSize += uint64(n) //增加待解包数据
}

func (this *PBReceiver) unpack() (interface{}, error) {
	var msg interface{}
	var err error
	msg, err = this.unPack()
	if nil != msg {
		return msg, err
	} else if nil != err {
		return nil, err
	}

	if this.unpackSize == 0 {
		this.unpackIdx = 0
		this.recvBuff = this.buffer
	} else if uint64(len(this.recvBuff)) < this.totalMaxPacket/4 {
		if this.unpackSize > 0 {
			//有数据尚未解包，需要移动到buffer前部
			copy(this.buffer, this.buffer[this.unpackIdx:this.unpackIdx+this.unpackSize])
		}
		this.recvBuff = this.buffer[this.unpackSize:]
		this.unpackIdx = 0
	}

	for k, _ := range this.recvBuff {
		this.recvBuff[k] = 0
	}

	return nil, nil
}

func TestPB2(t *testing.T) {

	go func() {
		http.ListenAndServe("0.0.0.0:6060", nil)
	}()

	pb.Register(&testproto.Test{}, 1)
	o := &testproto.Test{}
	o.A = proto.String("helloadsfe")
	o.B = proto.Int32(17)
	msg, _ := NewPbEncoder(1024).EnCode(o)

	var buff bytes.Buffer
	for i := 0; i < 372; i++ {
		buff.Write(msg.Bytes())
	}

	data := buff.Bytes()
	//22
	r := NewPBReceiver(4096)

	var v interface{}
	var err error

	var sendbuff []byte

	for {

		var n int

		if nil == sendbuff {
			sendbuff = data
		}

		if len(sendbuff) < 10 {
			n = len(sendbuff)
		} else {
			n = rand.Int() % len(sendbuff)
		}

		space := len(r.recvBuff)

		if n > space {
			n = space
		}

		r.append(sendbuff[:n])
		sendbuff = sendbuff[n:]
		if len(sendbuff) == 0 {
			sendbuff = nil
		}

		for {
			v, err = r.unpack()
			if nil != err {
				t.Fatal(err)
			}
			if nil == v {
				break
			}
		}
	}

}

/*
func TestPB1(t *testing.T) {
	pb.Register(&testproto.Test{}, 1)
	o := &testproto.Test{}
	o.A = proto.String("helloadsfe")
	o.B = proto.Int32(17)
	msg, _ := NewPbEncoder(1024).EnCode(o)
	bytes := msg.Bytes()
	//22
	r := NewPBReceiver(64)
	r.append(bytes)

	var v interface{}
	var err error

	v, _ = r.unpack()

	if v == nil {
		t.Fatal("v == nil")
	}

	v, _ = r.unpack()

	if v != nil {
		t.Fatal("v != nil")
	}

	if r.unpackIdx != 0 || r.unpackSize != 0 {
		t.Fatal("r.unpackIdx != 0 || r.unpackSize != 0")
	}

	r.append(bytes)
	r.append(bytes[:10])

	v, _ = r.unpack()

	if v == nil {
		t.Fatal("v == nil")
	}

	v, _ = r.unpack()

	if v != nil {
		t.Fatal("v != nil")
	}

	log.Println(r.unpackSize, r.unpackIdx)

	r.append(bytes[10:])
	r.append(bytes[:10])

	v, _ = r.unpack()

	if v == nil {
		t.Fatal("v == nil")
	}

	v, _ = r.unpack()

	if v != nil {
		t.Fatal("v != nil")
	}

	log.Println(r.unpackSize, r.unpackIdx)

	r.append(bytes[10:])
	r.append(bytes[:10])

	v, _ = r.unpack()

	if v == nil {
		t.Fatal("v == nil")
	}

	v, _ = r.unpack()

	if v != nil {
		t.Fatal("v != nil")
	}

	log.Println(r.unpackSize, r.unpackIdx)

	r.append(bytes[10:])
	r.append(bytes[:10])

	v, _ = r.unpack()

	if v == nil {
		t.Fatal("v == nil")
	}

	v, _ = r.unpack()

	if v != nil {
		t.Fatal("v != nil")
	}

	log.Println(r.unpackSize, r.unpackIdx)

	r.append(bytes[10:])
	r.append(bytes[:10])

	v, _ = r.unpack()

	if v == nil {
		t.Fatal("v == nil")
	}

	v, _ = r.unpack()

	if v != nil {
		t.Fatal("v != nil")
	}

	log.Println(r.unpackSize, r.unpackIdx)

}
*/
