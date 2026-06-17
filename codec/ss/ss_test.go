package ss

//go test -race -covermode=atomic -v -coverprofile=coverage.out -run=.
//go tool cover -html=coverage.out
import (
	"encoding/binary"
	"testing"

	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/codec/pb"
	"github.com/sniperHW/clustergo/rpc"
	"github.com/stretchr/testify/assert"
)

func init() {
	pb.Register(Namespace, &Echo{}, 1)
}

// payloadOf strips the 4-byte length prefix from a framed message and returns
// the payload, mirroring what socket.recvloop hands to Codec.Decode.
func payloadOf(frame []byte) []byte {
	payloadLen := int(binary.BigEndian.Uint32(frame[:4]))
	return frame[4 : 4+payloadLen]
}

func TestRPCResponse(t *testing.T) {
	selfAddr, _ := addr.MakeLogicAddr("1.1.1")
	targetAddr, _ := addr.MakeLogicAddr("1.1.2")
	codec := NewCodec(selfAddr)
	msg := NewMessage(targetAddr, selfAddr, &rpc.ResponseMsg{
		Seq: 1,
		Ret: []byte("world"),
	})
	frame, n := codec.Encode(nil, msg)
	assert.Equal(t, len(frame), n)

	dec := NewCodec(targetAddr)
	message, err := dec.Decode(payloadOf(frame))
	assert.Nil(t, err)
	rpcResp, ok := message.(*Message).Payload().(*rpc.ResponseMsg)
	assert.Equal(t, true, ok)
	assert.Equal(t, rpcResp.Seq, uint64(1))
	assert.Equal(t, rpcResp.Ret, []byte("world"))
}

func TestRPCRequest(t *testing.T) {
	selfAddr, _ := addr.MakeLogicAddr("1.1.1")
	targetAddr, _ := addr.MakeLogicAddr("1.1.2")
	codec := NewCodec(selfAddr)
	msg := NewMessage(targetAddr, selfAddr, &rpc.RequestMsg{
		Seq:    1,
		Method: "hello",
		Arg:    []byte("world"),
	})
	frame, n := codec.Encode(nil, msg)
	assert.Equal(t, len(frame), n)

	dec := NewCodec(targetAddr)
	message, err := dec.Decode(payloadOf(frame))
	assert.Nil(t, err)
	rpcReq, ok := message.(*Message).Payload().(*rpc.RequestMsg)
	assert.Equal(t, true, ok)
	assert.Equal(t, rpcReq.Seq, uint64(1))
	assert.Equal(t, rpcReq.Method, "hello")
	assert.Equal(t, rpcReq.Arg, []byte("world"))
}

func TestMessage(t *testing.T) {
	selfAddr, _ := addr.MakeLogicAddr("1.1.1")
	targetAddr, _ := addr.MakeLogicAddr("1.1.2")
	codec := NewCodec(selfAddr)
	msg := NewMessage(targetAddr, selfAddr, &Echo{Msg: "hello"})
	frame, n := codec.Encode(nil, msg)
	assert.Equal(t, len(frame), n)

	dec := NewCodec(targetAddr)
	message, err := dec.Decode(payloadOf(frame))
	assert.Nil(t, err)
	assert.Equal(t, message.(*Message).From(), selfAddr)
	assert.Equal(t, message.(*Message).To(), targetAddr)
	assert.Equal(t, message.(*Message).Cmd(), uint16(1))
	assert.Equal(t, message.(*Message).Payload().(*Echo).Msg, "hello")
}

func TestRelayMessage(t *testing.T) {
	selfAddr, _ := addr.MakeLogicAddr("1.1.1")
	harborAddr, _ := addr.MakeLogicAddr("1.255.1")
	targetAddr, _ := addr.MakeLogicAddr("1.1.3")
	codec := NewCodec(selfAddr)
	msg := NewMessage(targetAddr, selfAddr, &rpc.RequestMsg{
		Seq:    1,
		Method: "hello",
		Arg:    []byte("world"),
	})
	frame, n := codec.Encode(nil, msg)
	assert.Equal(t, len(frame), n)

	dec := NewCodec(harborAddr)
	message, err := dec.Decode(payloadOf(frame))
	assert.Nil(t, err)
	relayMessage, ok := message.(*RelayMessage)
	assert.Equal(t, ok, true)
	assert.Equal(t, n, len(relayMessage.Payload()))
	rpcReq := relayMessage.GetRpcRequest()
	assert.Equal(t, rpcReq.Seq, uint64(1))
	assert.Equal(t, rpcReq.Method, "hello")
	assert.Equal(t, rpcReq.Arg, []byte("world"))
}
