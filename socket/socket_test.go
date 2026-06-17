package socket

import (
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

// rawCodec frames []byte as [4-byte len][payload].
type rawCodec struct{}

func (rawCodec) Encode(dst []byte, o interface{}) ([]byte, int) {
	b := o.([]byte)
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(b)))
	dst = append(dst, hdr[:]...)
	dst = append(dst, b...)
	return dst, 4 + len(b)
}

func (rawCodec) Decode(payload []byte) (interface{}, error) {
	out := make([]byte, len(payload))
	copy(out, payload)
	return out, nil
}

func TestSocket_SendRecv(t *testing.T) {
	received := make(chan []byte, 4)
	srvClosed := make(chan error, 1)

	ln, serve, err := ListenTCP("127.0.0.1:0", func(conn *net.TCPConn) {
		srv := New(conn, rawCodec{}, Options{SendChanSize: 8, AutoRecv: true})
		srv.SetPacketHandler(func(_ context.Context, p interface{}) error {
			received <- p.([]byte)
			return nil
		}).SetCloseCallback(func(err error) { srvClosed <- err })
		srv.Recv()
	})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go serve()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	cli := New(conn.(*net.TCPConn), rawCodec{}, Options{SendChanSize: 8})

	if err := cli.Send([]byte("hello"), time.Time{}); err != nil {
		t.Fatalf("send1: %v", err)
	}
	if err := cli.Send([]byte("world"), time.Time{}); err != nil {
		t.Fatalf("send2: %v", err)
	}

	for i, want := range []string{"hello", "world"} {
		select {
		case got := <-received:
			if string(got) != want {
				t.Fatalf("msg %d: got %q want %q", i, got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("msg %d timeout", i)
		}
	}

	cli.Close(nil)
	select {
	case <-srvClosed:
	case <-time.After(time.Second):
		t.Fatal("server close callback not fired")
	}
}
