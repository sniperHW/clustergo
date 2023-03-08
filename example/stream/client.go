package main

import (
	"context"
	"encoding/binary"
	"errors"
	"log"
	"net"
	"sync/atomic"
	"time"

	"github.com/sniperHW/clustergo/example/stream/pb"
	"github.com/sniperHW/netgo"
	"google.golang.org/protobuf/proto"
)

type PBCodec struct {
	r    int
	w    int
	buff []byte
}

func (codec *PBCodec) Decode(b []byte) (interface{}, error) {
	o := &pb.Echo{}
	if err := proto.Unmarshal(b, o); nil != err {
		return nil, err
	} else {
		return o, nil
	}
}

func (codec *PBCodec) Encode(buffs net.Buffers, o interface{}) (net.Buffers, int) {
	if _, ok := o.(*pb.Echo); !ok {
		return buffs, 0
	} else {
		if data, err := proto.Marshal(o.(*pb.Echo)); nil != err {
			return buffs, 0
		} else {
			bu := make([]byte, 4)
			binary.BigEndian.PutUint32(bu, uint32(len(data)))
			return append(buffs, bu, data), len(bu) + len(data)
		}
	}
}

func (codec *PBCodec) read(readable netgo.ReadAble, deadline time.Time) (int, error) {
	if err := readable.SetReadDeadline(deadline); err != nil {
		return 0, err
	} else {
		return readable.Read(codec.buff[codec.w:])
	}
}

func (codec *PBCodec) Recv(readable netgo.ReadAble, deadline time.Time) (pkt []byte, err error) {
	const lenHead int = 4
	for {
		rr := codec.r
		pktLen := 0
		if (codec.w - rr) >= lenHead {
			pktLen = int(binary.BigEndian.Uint32(codec.buff[rr:]))
			rr += lenHead
		}

		if pktLen > 0 {
			if pktLen > (len(codec.buff) - lenHead) {
				err = errors.New("pkt too large")
				return
			}
			if (codec.w - rr) >= pktLen {
				pkt = codec.buff[rr : rr+pktLen]
				rr += pktLen
				codec.r = rr
				if codec.r == codec.w {
					codec.r = 0
					codec.w = 0
				}
				return
			}
		}

		if codec.r > 0 {
			//移动到头部
			copy(codec.buff, codec.buff[codec.r:codec.w])
			codec.w = codec.w - codec.r
			codec.r = 0
		}

		var n int
		n, err = codec.read(readable, deadline)
		if n > 0 {
			codec.w += n
		}
		if nil != err {
			return
		}
	}
}

func main() {
	dialer := &net.Dialer{}
	var (
		s netgo.Socket
	)

	codec := &PBCodec{buff: make([]byte, 4096)}

	for {
		if conn, err := dialer.Dial("tcp", "127.0.0.1:18113"); nil != err {
			time.Sleep(time.Second)
		} else {
			s = netgo.NewTcpSocket(conn.(*net.TCPConn), codec)
			break
		}
	}

	okChan := make(chan struct{})
	count := int32(0)

	as := netgo.NewAsynSocket(s, netgo.AsynSocketOption{
		Codec: codec,
	}).SetCloseCallback(func(_ *netgo.AsynSocket, err error) {
		log.Println("client closed err:", err)
	}).SetPacketHandler(func(_ context.Context, as *netgo.AsynSocket, packet interface{}) error {
		c := atomic.AddInt32(&count, 1)
		log.Println("go echo resp", c)
		if c == 100 {
			close(okChan)
		} else {
			as.Recv()
		}
		return nil
	}).Recv()

	for i := 0; i < 100; i++ {
		as.Send(&pb.Echo{Msg: "hello"})
	}
	<-okChan
	as.Close(nil)
}
