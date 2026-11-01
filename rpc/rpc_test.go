package rpc

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

type jsonCodec struct{}

func (jsonCodec) Encode(dst []byte, v interface{}) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return dst, err
	}
	return append(dst, b...), nil
}
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
	server := NewServer()
	client := NewClient()
	return client, server, &loopChannel{name: "loop", client: client, server: server}
}

func TestRPC_Call(t *testing.T) {
	client, server, ch := newPair()
	server.Register("echo", func(ctx context.Context, r *Replyer, arg *echoArg) {
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
	server.Register("echo", func(ctx context.Context, r *Replyer, arg *echoArg) {
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
	server.Register("fire", func(ctx context.Context, r *Replyer, arg *echoArg) {
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
	server.Register("fail", func(ctx context.Context, r *Replyer, arg *echoArg) {
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
	codec := jsonCodec{}
	want := &echoArg{Msg: "arg-bytes"}
	req := &RequestMsg{
		Seq:      42,
		Method:   "hello",
		Arg:      want,
		UserData: []byte("ud"),
		Oneway:   true,
	}
	b, err := EncodeRequest(nil, req, codec)
	if err != nil {
		t.Fatalf("EncodeRequest: %v", err)
	}
	got, err := DecodeRequest(b, codec)
	if err != nil {
		t.Fatalf("DecodeRequest: %v", err)
	}
	if got.Seq != req.Seq || got.Method != req.Method || got.Oneway != req.Oneway ||
		string(got.UserData) != string(req.UserData) {
		t.Fatalf("header round-trip mismatch: got %+v want %+v", got, req)
	}
	var a echoArg
	if err := got.decodeArgInto(&a); err != nil {
		t.Fatalf("decodeArgInto: %v", err)
	}
	if a.Msg != want.Msg {
		t.Fatalf("arg round-trip mismatch: got %+v want %+v", a, want)
	}
}

func TestRPC_EncodeDecodeResponse(t *testing.T) {
	codec := jsonCodec{}
	// without error
	want := &echoRet{Msg: "ret"}
	resp := &ResponseMsg{Seq: 7, Ret: want}
	b, err := EncodeResponse(nil, resp, codec)
	if err != nil {
		t.Fatalf("EncodeResponse: %v", err)
	}
	got, err := DecodeResponse(b, codec)
	if err != nil {
		t.Fatalf("DecodeResponse: %v", err)
	}
	if got.Seq != resp.Seq || got.Err != nil {
		t.Fatalf("round-trip mismatch (no-err): got %+v", got)
	}
	var r echoRet
	if err := got.decodeRet(&r); err != nil {
		t.Fatalf("decodeRet: %v", err)
	}
	if r.Msg != want.Msg {
		t.Fatalf("ret round-trip mismatch: got %+v want %+v", r, want)
	}
	// with error
	respErr := &ResponseMsg{Seq: 9, Err: NewError(ErrMethod, "boom")}
	b, err = EncodeResponse(nil, respErr, codec)
	if err != nil {
		t.Fatalf("EncodeResponse(err): %v", err)
	}
	got, err = DecodeResponse(b, codec)
	if err != nil {
		t.Fatalf("DecodeResponse(err): %v", err)
	}
	if got.Err == nil || !got.Err.IsCode(ErrMethod) || got.Err.Error() != "boom" {
		t.Fatalf("err round-trip mismatch: got %+v", got.Err)
	}
}

func TestRPC_DecodeTruncated(t *testing.T) {
	if _, err := DecodeRequest(nil, nil); err == nil {
		t.Fatal("expected error decoding nil request")
	}
	if _, err := DecodeRequest([]byte{1, 2, 3}, nil); err == nil { // shorter than seq(8)
		t.Fatal("expected error decoding truncated request")
	}
}

// neverReply is a method handler that never calls Reply, so the client must rely on ctx.
func TestRPC_CallCancelled(t *testing.T) {
	client, server, ch := newPair()
	server.Register("hang", func(ctx context.Context, r *Replyer, arg *echoArg) {
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
	server.Register("hang", func(ctx context.Context, r *Replyer, arg *echoArg) {
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

// TestRPC_AsyncPool 验证 AsyncPool：慢方法在 worker 上异步执行，不阻塞调用方，
// 执行完成后结果仍能正确返回。
func TestRPC_AsyncPool(t *testing.T) {
	client, server, ch := newPair()
	// 1 个 worker、队列容量 2：可同时容纳 2 个待处理任务。
	pool := NewAsyncPool(1, 2)
	gate := make(chan struct{})
	started := make(chan struct{}, 1)

	server.Register("echo", pool.Wrap(func(ctx context.Context, r *Replyer, arg *echoArg) {
		started <- struct{}{} // 方法已被派发到 worker 上执行
		<-gate                // 模拟耗时，占住 worker
		r.Reply(&echoRet{Msg: arg.Msg})
	}))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var ret echoRet
	done := make(chan error, 1)
	go func() { done <- client.Call(ctx, ch, "echo", &echoArg{Msg: "hi"}, &ret) }()

	// 方法被 gate 阻塞期间，接收协程已返回，调用方只是在等响应。
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("method did not run on worker")
	}
	select {
	case err := <-done:
		t.Fatalf("call should still be pending while method runs, got err=%v", err)
	default:
	}

	close(gate) // 放行 worker
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Call: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("call did not return")
	}
	if ret.Msg != "hi" {
		t.Fatalf("unexpected ret: %+v", ret)
	}
}

// TestRPC_AsyncPoolBusy 验证队列满时立即回复 ErrBusy：先用 gate 占住唯一的 worker，
// 再同步地入队一个任务填满队列，第三次请求必然被拒绝。
func TestRPC_AsyncPoolBusy(t *testing.T) {
	pool := NewAsyncPool(1, 1) // 1 worker、队列容量 1：worker + 队列共可容纳 2 个任务
	gate := make(chan struct{})
	started := make(chan struct{}, 1)
	var wg sync.WaitGroup

	wrapped := pool.Wrap(func(ctx context.Context, r *Replyer, arg *echoArg) {
		defer wg.Done()
		if arg.Msg == "1" {
			started <- struct{}{} // 通知 worker 已被占住
		}
		<-gate // 占住唯一的 worker
		r.Reply(&echoRet{Msg: arg.Msg})
	})

	// 第 1 个任务：进入 worker 执行并阻塞，等它就位后 worker 才算被占满。
	wg.Add(1)
	go wrapped(context.Background(), &Replyer{req: &RequestMsg{Oneway: true}}, &echoArg{Msg: "1"})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}

	// 第 2 个任务：worker 已被占住，这里只会入队并立即返回，队列随之填满。
	wg.Add(1)
	wrapped(context.Background(), &Replyer{req: &RequestMsg{Oneway: true}}, &echoArg{Msg: "2"})

	// 第 3 个任务：worker 与队列均满，应立即收到 ErrBusy。
	errs := make(chan *Error, 1)
	wrapped(context.Background(), &Replyer{
		channel: &errCollectChannel{onErr: func(e *Error) { errs <- e }},
		req:     &RequestMsg{},
	}, &echoArg{Msg: "3"})

	select {
	case e := <-errs:
		if !e.IsCode(ErrBusy) {
			t.Fatalf("expected ErrBusy, got %v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("expected ErrBusy, timed out")
	}

	close(gate) // 放行 worker
	wg.Wait()   // 等前两个任务执行完，避免遗留阻塞的 goroutine
}

// TestRPC_AsyncPoolShared 验证多个方法（不同 Arg 类型）共享同一个池：
// 共享的池满时，另一个方法也应收到 ErrBusy。
func TestRPC_AsyncPoolShared(t *testing.T) {
	client, server, ch := newPair()
	// 1 worker、队列容量 1：worker + 队列共可容纳 2 个任务。
	pool := NewAsyncPool(1, 1)
	gate := make(chan struct{})
	started := make(chan struct{}, 1)
	var wg sync.WaitGroup

	// echo 方法阻塞 worker。
	server.Register("echo", pool.Wrap(func(ctx context.Context, r *Replyer, arg *echoArg) {
		defer wg.Done()
		if arg.Msg == "a" {
			started <- struct{}{} // 通知 worker 已被占住
		}
		<-gate
		r.Reply(&echoRet{Msg: arg.Msg})
	}))
	// fire 方法与 echo 共享同一个池。
	server.Register("fire", pool.Wrap(func(ctx context.Context, r *Replyer, arg *echoArg) {
		r.Reply(&echoRet{Msg: arg.Msg})
	}))

	// 第 1 个 echo 请求占住 worker（Oneway 不等待回复）。
	wg.Add(1)
	go server.OnMessage(context.Background(), ch, &RequestMsg{Seq: 1, Method: "echo", Oneway: true, Arg: &echoArg{Msg: "a"}})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}

	// 第 2 个 echo 请求只会入队并立即返回，共享队列被填满。
	wg.Add(1)
	server.OnMessage(context.Background(), ch, &RequestMsg{Seq: 2, Method: "echo", Oneway: true, Arg: &echoArg{Msg: "b"}})

	// 第 3 个请求走 fire（与 echo 共享池），应因池满收到 ErrBusy。
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var ret echoRet
	err := client.Call(ctx, ch, "fire", &echoArg{Msg: "c"}, &ret)
	if e, ok := err.(*Error); !ok || !e.IsCode(ErrBusy) {
		t.Fatalf("expected ErrBusy from shared pool, got %v", err)
	}

	close(gate) // 放行 worker
	wg.Wait()   // 等前两个任务执行完，避免遗留阻塞的 goroutine
}

// errCollectChannel 是一个只用于收集 Error 回复的测试 channel。
type errCollectChannel struct {
	onErr func(*Error)
}

func (c *errCollectChannel) Request(*RequestMsg) error                             { return nil }
func (c *errCollectChannel) RequestWithContext(context.Context, *RequestMsg) error { return nil }
func (c *errCollectChannel) Reply(resp *ResponseMsg) error {
	if resp.Err != nil && c.onErr != nil {
		c.onErr(resp.Err)
	}
	return nil
}
func (c *errCollectChannel) Name() string                { return "err-collect" }
func (c *errCollectChannel) IsRetryAbleError(error) bool { return false }
