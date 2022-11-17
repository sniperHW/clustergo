package ss

//go test -race -covermode=atomic -v -coverprofile=coverage.out -run=.
//go tool cover -html=coverage.out
import (
	"log"
	"net"
	"testing"
	"time"

	"github.com/sniperHW/rpcgo"
	"github.com/sniperHW/clustergo/addr"

	"github.com/sniperHW/clustergo/codec/pb"
	"github.com/stretchr/testify/assert"
)

func init() {
	pb.Register(Namespace, &Echo{}, 1)
}

type readable struct {
	buff []byte
}

func (r *readable) Read(buff []byte) (int, error) {
	copy(buff, r.buff)
	return len(r.buff), nil
}

func (r *readable) SetReadDeadline(_ time.Time) error {
	return nil
}

func TestRPCResponse(t *testing.T) {

	selfAddr, _ := addr.MakeLogicAddr("1.1.1")
	targetAddr, _ := addr.MakeLogicAddr("1.1.2")
	var buffs net.Buffers
	var n int
	{
		codec := NewCodec(selfAddr)
		msg := NewMessage(targetAddr, selfAddr, &rpcgo.ResponseMsg{
			Seq: 1,
			Ret: []byte("world"),
		})

		buffs, n = codec.Encode(buffs, msg)
		log.Println(n)
		assert.Equal(t, len(buffs[0]), n)
		log.Println(buffs[0])
	}

	{
		codec := NewCodec(targetAddr)

		r := &readable{
			buff: buffs[0],
		}

		pkt, err := codec.Recv(r, time.Time{})
		assert.Nil(t, err)
		assert.Equal(t, len(pkt), n-4)

		message, err := codec.Decode(pkt)
		assert.Nil(t, err)

		rpcReq, ok := message.(*Message).Payload().(*rpcgo.ResponseMsg)
		assert.Equal(t, true, ok)
		assert.Equal(t, rpcReq.Seq, uint64(1))
		assert.Equal(t, rpcReq.Ret, []byte("world"))
	}

}

func TestRPCRequest(t *testing.T) {

	selfAddr, _ := addr.MakeLogicAddr("1.1.1")
	targetAddr, _ := addr.MakeLogicAddr("1.1.2")
	var buffs net.Buffers
	var n int
	{
		codec := NewCodec(selfAddr)
		msg := NewMessage(targetAddr, selfAddr, &rpcgo.RequestMsg{
			Seq:    1,
			Method: "hello",
			Arg:    []byte("world"),
		})

		buffs, n = codec.Encode(buffs, msg)
		log.Println(n)
		assert.Equal(t, len(buffs[0]), n)
		log.Println(buffs[0])
	}

	{
		codec := NewCodec(targetAddr)

		r := &readable{
			buff: buffs[0],
		}

		pkt, err := codec.Recv(r, time.Time{})
		assert.Nil(t, err)
		assert.Equal(t, len(pkt), n-4)

		message, err := codec.Decode(pkt)
		assert.Nil(t, err)

		rpcReq, ok := message.(*Message).Payload().(*rpcgo.RequestMsg)
		assert.Equal(t, true, ok)
		assert.Equal(t, rpcReq.Seq, uint64(1))
		assert.Equal(t, rpcReq.Method, "hello")
		assert.Equal(t, rpcReq.Arg, []byte("world"))
	}

}

func TestMessage(t *testing.T) {
	selfAddr, _ := addr.MakeLogicAddr("1.1.1")
	targetAddr, _ := addr.MakeLogicAddr("1.1.2")
	var buffs net.Buffers
	var n int
	{
		codec := NewCodec(selfAddr)

		msg := NewMessage(targetAddr, selfAddr, &Echo{Msg: "hello"})

		buffs, n = codec.Encode(buffs, msg)
		log.Println(n)
		assert.Equal(t, len(buffs[0]), n)
		log.Println(buffs[0])
	}

	{
		codec := NewCodec(targetAddr)

		r := &readable{
			buff: buffs[0],
		}

		pkt, err := codec.Recv(r, time.Time{})
		assert.Nil(t, err)
		assert.Equal(t, len(pkt), n-4)

		message, err := codec.Decode(pkt)
		assert.Nil(t, err)

		assert.Equal(t, message.(*Message).From(), selfAddr)
		assert.Equal(t, message.(*Message).To(), targetAddr)
		assert.Equal(t, message.(*Message).Cmd(), uint16(1))
		assert.Equal(t, message.(*Message).Payload().(*Echo).Msg, "hello")
	}

}

func TestRelayMessage(t *testing.T) {

	selfAddr, _ := addr.MakeLogicAddr("1.1.1")
	harborAddr, _ := addr.MakeLogicAddr("1.255.1")
	targetAddr, _ := addr.MakeLogicAddr("1.1.3")
	var buffs net.Buffers
	var n int
	{
		codec := NewCodec(selfAddr)
		msg := NewMessage(targetAddr, selfAddr, &rpcgo.RequestMsg{
			Seq:    1,
			Method: "hello",
			Arg:    []byte("world"),
		})

		buffs, n = codec.Encode(buffs, msg)
		log.Println(n)
		assert.Equal(t, len(buffs[0]), n)
		log.Println(buffs[0])
	}

	{
		codec := NewCodec(harborAddr)

		r := &readable{
			buff: buffs[0],
		}

		pkt, err := codec.Recv(r, time.Time{})
		assert.Nil(t, err)
		assert.Equal(t, len(pkt), n-4)

		message, err := codec.Decode(pkt)
		assert.Nil(t, err)

		relayMessage, ok := message.(*RelayMessage)
		assert.Equal(t, ok, true)
		assert.Equal(t, n, len(relayMessage.Payload()))
		log.Println(relayMessage.Payload())

		rpcReq := relayMessage.GetRpcRequest()
		assert.Equal(t, rpcReq.Seq, uint64(1))
		assert.Equal(t, rpcReq.Method, "hello")
		assert.Equal(t, rpcReq.Arg, []byte("world"))
	}

}
