package rpc

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

type jsonCodec struct{}

func (jsonCodec) Encode(v interface{}) ([]byte, error) { return json.Marshal(v) }
func (jsonCodec) Decode(b []byte, v interface{}) error { return json.Unmarshal(b, v) }

type echoArg struct{ Msg string }
type echoRet struct{ Msg string }

// loopChannel wires a Client and Server in-process:
//
//	Request* -> server.OnMessage (async); Reply -> client.OnMessage.
type loopChannel struct {
	name   string
	client *Client
	server *Server
}

func (c *loopChannel) Request(req *RequestMsg) error {
	go c.server.OnMessage(context.Background(), c, req)
	return nil
}
func (c *loopChannel) RequestWithContext(ctx context.Context, req *RequestMsg) error {
	go c.server.OnMessage(ctx, c, req)
	return nil
}
func (c *loopChannel) Reply(resp *ResponseMsg) error {
	c.client.OnMessage(resp)
	return nil
}
func (c *loopChannel) Name() string                { return c.name }
func (c *loopChannel) IsRetryAbleError(error) bool { return false }

func newPair() (*Client, *Server, *loopChannel) {
	codec := jsonCodec{}
	server := NewServer(codec)
	client := NewClient(codec)
	return client, server, &loopChannel{name: "loop", client: client, server: server}
}

func TestRPC_Call(t *testing.T) {
	client, server, ch := newPair()
	Register(server, "echo", func(ctx context.Context, r *Replyer, arg *echoArg) {
		r.Reply(&echoRet{Msg: arg.Msg})
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var ret echoRet
	if err := client.Call(ctx, ch, "echo", &echoArg{Msg: "hi"}, &ret); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if ret.Msg != "hi" {
		t.Fatalf("unexpected ret: %+v", ret)
	}
}

func TestRPC_AsyncCall(t *testing.T) {
	client, server, ch := newPair()
	Register(server, "echo", func(ctx context.Context, r *Replyer, arg *echoArg) {
		r.Reply(&echoRet{Msg: arg.Msg})
	})
	done := make(chan error, 1)
	var got echoRet
	if err := client.AsyncCall(ch, "echo", &echoArg{Msg: "yo"}, &got, time.Now().Add(time.Second), func(r interface{}, e error) {
		if e != nil {
			done <- e
			return
		}
		got = *(r.(*echoRet))
		done <- nil
	}); err != nil {
		t.Fatalf("AsyncCall: %v", err)
	}
	select {
	case e := <-done:
		if e != nil {
			t.Fatalf("callback err: %v", e)
		}
		if got.Msg != "yo" {
			t.Fatalf("unexpected: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestRPC_Post(t *testing.T) {
	client, server, ch := newPair()
	called := make(chan struct{}, 1)
	Register(server, "fire", func(ctx context.Context, r *Replyer, arg *echoArg) {
		called <- struct{}{}
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.Call(ctx, ch, "fire", &echoArg{Msg: "x"}, nil); err != nil { // nil ret => oneway
		t.Fatalf("Post: %v", err)
	}
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("oneway not delivered")
	}
}

func TestRPC_Error(t *testing.T) {
	client, server, ch := newPair()
	Register(server, "fail", func(ctx context.Context, r *Replyer, arg *echoArg) {
		r.Error(NewError(ErrMethod, "boom"))
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var ret echoRet
	if err := client.Call(ctx, ch, "fail", &echoArg{}, &ret); err == nil {
		t.Fatal("expected error")
	}
}

func TestRPC_EncodeDecodeRequest(t *testing.T) {
	req := &RequestMsg{
		Seq:      42,
		Method:   "hello",
		Arg:      []byte("arg-bytes"),
		UserData: []byte("ud"),
		Oneway:   true,
	}
	got, err := DecodeRequest(EncodeRequest(req))
	if err != nil {
		t.Fatalf("DecodeRequest: %v", err)
	}
	if got.Seq != req.Seq || got.Method != req.Method || string(got.Arg) != string(req.Arg) ||
		string(got.UserData) != string(req.UserData) || got.Oneway != req.Oneway {
		t.Fatalf("round-trip mismatch: got %+v want %+v", got, req)
	}
}

func TestRPC_EncodeDecodeResponse(t *testing.T) {
	// without error
	resp := &ResponseMsg{Seq: 7, Ret: []byte("ret")}
	got, err := DecodeResponse(EncodeResponse(resp))
	if err != nil {
		t.Fatalf("DecodeResponse: %v", err)
	}
	if got.Seq != resp.Seq || string(got.Ret) != string(resp.Ret) || got.Err != nil {
		t.Fatalf("round-trip mismatch (no-err): got %+v", got)
	}
	// with error
	respErr := &ResponseMsg{Seq: 9, Err: NewError(ErrMethod, "boom")}
	got, err = DecodeResponse(EncodeResponse(respErr))
	if err != nil {
		t.Fatalf("DecodeResponse(err): %v", err)
	}
	if got.Err == nil || !got.Err.IsCode(ErrMethod) || got.Err.Error() != "boom" {
		t.Fatalf("err round-trip mismatch: got %+v", got.Err)
	}
}

func TestRPC_DecodeTruncated(t *testing.T) {
	if _, err := DecodeRequest(nil); err == nil {
		t.Fatal("expected error decoding nil request")
	}
	if _, err := DecodeRequest([]byte{1, 2, 3}); err == nil { // shorter than seq(8)
		t.Fatal("expected error decoding truncated request")
	}
}

// neverReply is a method handler that never calls Reply, so the client must rely on ctx.
func TestRPC_CallCancelled(t *testing.T) {
	client, server, ch := newPair()
	Register(server, "hang", func(ctx context.Context, r *Replyer, arg *echoArg) {
		// intentionally never reply
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	var ret echoRet
	err := client.Call(ctx, ch, "hang", &echoArg{Msg: "x"}, &ret)
	if err == nil {
		t.Fatal("expected error")
	}
	if e, ok := err.(*Error); !ok || !e.IsCode(ErrCancel) {
		t.Fatalf("expected ErrCancel, got %v", err)
	}
}

func TestRPC_CallTimeout(t *testing.T) {
	client, server, ch := newPair()
	Register(server, "hang", func(ctx context.Context, r *Replyer, arg *echoArg) {
		// intentionally never reply
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	var ret echoRet
	err := client.Call(ctx, ch, "hang", &echoArg{Msg: "x"}, &ret)
	if err == nil {
		t.Fatal("expected error")
	}
	if e, ok := err.(*Error); !ok || !e.IsCode(ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
}
