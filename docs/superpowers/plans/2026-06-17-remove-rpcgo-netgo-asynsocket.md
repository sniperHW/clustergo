# Remove rpcgo + netgo.AsynSocket, poolbuff-gather send — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove clustergo's dependency on `rpcgo` and `netgo.AsynSocket` by inlining a simplified RPC into `clustergo/rpc` and replacing the async socket with a TCP-only `clustergo/socket` that sends via a `netgo/poolbuff` gather buffer.

**Architecture:** Bottom-up. (1) New `clustergo/rpc` = inlined, simplified rpcgo (stdlib-only). (2) New `clustergo/socket` = TCP-only async socket, poolbuff gather-send, owns length-prefix framing. (3) Atomic switch: `codec/ss` implements `socket.Codec`, root package (`node.go`/`rpc.go`/`cluster.go`/`logger.go`) rewires to `rpc`+`socket`, tests + `example/pbrpc` updated, `codec/receiver.go` deleted. (4) `go.mod` tidy drops rpcgo. `netgo` stays, imported only for `poolbuff`.

**Tech Stack:** Go 1.25, stdlib `net`, `github.com/sniperHW/netgo/poolbuff`, `google.golang.org/protobuf`, `github.com/xtaci/smux`.

**Reference spec:** `docs/superpowers/specs/2026-06-17-remove-rpcgo-netgo-asynsocket-design.md`

**rpcgo source location (for the port in Task 1):** `/Users/huangwei/go_work/src/github.com/sniperHW/rpcgo/`

---

## File Structure

**New packages:**
- `rpc/` — inlined simplified rpcgo: `logger.go`, `bytes.go`, `rpc.go`, `server.go`, `client.go`, `rpc_test.go`. Responsibility: RPC request/response framing, Client/Server/Replyer, arg/ret Codec, generic `Register[Arg]`. Depends on: stdlib only.
- `socket/` — `socket.go`, `socket_test.go`. Responsibility: TCP-only async socket; poolbuff gather-send; length-prefix recv framing. Depends on: `netgo/poolbuff`.

**Modified:**
- `codec/ss/codec.go` — `SSCodec` implements `socket.Codec`; append-style `Encode`; drops embedded receiver.
- `codec/ss/message.go` — `rpcgo` → `rpc`; removes local `MaxPacketSize`.
- `node.go`, `rpc.go`, `cluster.go`, `logger.go` — `rpcgo`→`rpc`, `netgo.*`→`socket.*`.
- `cluster_test.go`, `codec/ss/ss_test.go` — `rpcgo` → `rpc`.
- `example/pbrpc/{genrpc/gen.go, service/echo/echo.go, service/test/test.go}` — `rpcgo` → `rpc`.
- `go.mod` — drop `rpcgo`.

**Deleted:**
- `codec/receiver.go` — framing moved into `socket`.

**Untouched:** `addr/`, `membership/`, `pkg/crypto`, `codec/buffer`, `codec/pb`, `example/stream`, `example/membership`.

---

## Task 1: Create `clustergo/rpc` package (inlined, simplified rpcgo)

**Files:**
- Create: `rpc/logger.go`, `rpc/bytes.go`, `rpc/rpc.go`, `rpc/server.go`, `rpc/client.go`
- Test: `rpc/rpc_test.go`

This package has no dependency on socket or the root package, so it builds and tests in isolation. The root package still uses `rpcgo` until Task 3 — that's fine.

- [ ] **Step 1: Create `rpc/logger.go`**

```go
package rpc

// Logger is the logging interface used by the rpc package. It matches
// clustergo.Logger exactly so the same logger instance can be shared.
type Logger interface {
	Debugf(string, ...interface{})
	Infof(string, ...interface{})
	Warnf(string, ...interface{})
	Errorf(string, ...interface{})
	Panicf(string, ...interface{})
	Fatalf(string, ...interface{})
	Debug(...interface{})
	Info(...interface{})
	Warn(...interface{})
	Error(...interface{})
	Panic(...interface{})
	Fatal(...interface{})
}

var logger Logger

// InitLogger sets the package logger. Call from clustergo.InitLogger.
func InitLogger(l Logger) {
	logger = l
}
```

- [ ] **Step 2: Create `rpc/bytes.go` (verbatim port)**

Copy `/Users/huangwei/go_work/src/github.com/sniperHW/rpcgo/bytes.go` to `rpc/bytes.go` and change only the package declaration from `package rpcgo` to `package rpc`. No other changes. (It defines `BytesWriter` and `BytesReader` with their Write*/Read* methods.)

- [ ] **Step 3: Create `rpc/rpc.go` (port with deletions)**

Copy `/Users/huangwei/go_work/src/github.com/sniperHW/rpcgo/rpc.go` to `rpc/rpc.go`, then apply these edits:

1. Change `package rpcgo` → `package rpc`.
2. **Delete** the logger var/InitLogger block (these moved to `logger.go`):
```go
var logger Logger

func InitLogger(l Logger) {
	logger = l
}
```
3. **Delete** the unused disconnect-fast-fail interfaces and their comment (the whole block):
```go
/*
 *  默认情况下不关注channel的断开事件，如果需要在channel断开的时候
 *  快速中断等待中的RPC，使用者需要额外实现ChannelInterestDisconnect接口，自己管理调用上下文
 */

type PendingCall interface {
	OnDisconnect()
}

type ChannelInterestDisconnect interface {
	OnDisconnect()
	PutPending(uint64, PendingCall)
	LoadAndDeletePending(uint64) (interface{}, bool)
}
```
4. Keep everything else verbatim: `Error`/`NewError`, the error-code `const` block (`ErrOk`…`errEnd`), `RequestMsg`/`ResponseMsg` + `GetArg`, the length consts, `EncodeRequest`/`DecodeRequest`/`EncodeResponse`/`DecodeResponse`, the `Codec` interface, the `Channel` interface, `Register[Arg]`, `UnRegister`.

- [ ] **Step 4: Create `rpc/server.go` (verbatim port)**

Copy `/Users/huangwei/go_work/src/github.com/sniperHW/rpcgo/server.go` to `rpc/server.go`, change `package rpcgo` → `package rpc`. No other changes. (Defines `method`, `Replyer`, `Server`, `NewServer`, `SetInInterceptor`, `Stop`, `OnMessage`.)

- [ ] **Step 5: Create `rpc/client.go` (simplified)**

This is the heavily-edited file: `ChannelInterestDisconnect` branching is removed and `OnMessage` always uses the 32-shard pending map. Create `rpc/client.go` with this complete content:

```go
package rpc

import (
	"context"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

var syncCtxPool = sync.Pool{
	New: func() interface{} { return syncContext(make(chan *ResponseMsg, 1)) },
}

var asynCtxPool = sync.Pool{
	New: func() interface{} { return &asynContext{} },
}

type syncContext chan *ResponseMsg

type asynContext struct {
	onResponse func(interface{}, error)
	timer      *time.Timer
	fired      int32
	ret        interface{}
	req        *RequestMsg
	cli        *Client
}

func (c *asynContext) callOnResponse(codec Codec, resp []byte, err *Error) bool {
	if atomic.CompareAndSwapInt32(&c.fired, 0, 1) {
		var e error
		if err == nil {
			if e = codec.Decode(resp, c.ret); e != nil {
				logger.Errorf("callOnResponse decode error:%v", e)
				e = NewError(ErrOther, "decode resp.Ret")
			}
		} else {
			e = err
		}
		c.cli.callInInterceptor(c.req, c.ret, e)
		c.onResponse(c.ret, e)
		return true
	} else {
		return false
	}
}

func (c *asynContext) onTimeout() {
	if c.callOnResponse(nil, nil, NewError(ErrTimeout, "timeout")) {
		asynCtxPool.Put(c)
	}
}

type Client struct {
	sync.Mutex
	nextSequence   uint32
	timestamp      uint32
	timeOffset     uint32
	startTime      time.Time
	codec          Codec
	pendingCall    [32]sync.Map
	inInterceptor  []func(*RequestMsg, interface{}, error) //入站管道线
	outInterceptor []func(*RequestMsg, interface{})        //出站管道线
}

func NewClient(codec Codec) *Client {
	return &Client{
		codec:      codec,
		timeOffset: uint32(time.Now().Unix() - time.Date(2023, time.January, 1, 0, 0, 0, 0, time.Local).Unix()),
		startTime:  time.Now(),
	}
}

func (c *Client) SetInInterceptor(interceptor []func(*RequestMsg, interface{}, error)) {
	c.inInterceptor = interceptor
}

func (c *Client) SetOutInterceptor(interceptor []func(*RequestMsg, interface{})) {
	c.outInterceptor = interceptor
}

func (c *Client) callInInterceptor(req *RequestMsg, ret interface{}, err error) {
	for _, fn := range c.inInterceptor {
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("%v: %s", r, debug.Stack())
				}
			}()
			fn(req, ret, err)
		}()
	}
}

func (c *Client) callOutInterceptor(req *RequestMsg, arg interface{}) {
	for _, fn := range c.outInterceptor {
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("%v: %s", r, debug.Stack())
				}
			}()
			fn(req, arg)
		}()
	}
}

func (c *Client) getTimeStamp() uint32 {
	return uint32(time.Since(c.startTime)/time.Second) + c.timeOffset
}

func (c *Client) makeSequence() (seq uint64) {
	timestamp := c.getTimeStamp()
	c.Lock()
	if timestamp > c.timestamp {
		c.timestamp = timestamp
		c.nextSequence = 1
	} else {
		c.nextSequence++
	}
	seq = uint64(c.timestamp)<<32 + uint64(c.nextSequence)
	c.Unlock()
	return seq
}

// pending returns the shard map for a sequence number.
func (c *Client) pending(seq uint64) *sync.Map {
	return &c.pendingCall[int(seq)%len(c.pendingCall)]
}

func (c *Client) OnMessage(resp *ResponseMsg) {
	ctx, ok := c.pending(resp.Seq).LoadAndDelete(resp.Seq)
	if ok {
		switch v := ctx.(type) {
		case syncContext:
			v <- resp
		case *asynContext:
			ok := v.timer.Stop()
			v.callOnResponse(c.codec, resp.Ret, resp.Err)
			if ok {
				asynCtxPool.Put(v)
			}
		}
	} else {
		logger.Infof("onResponse with no reqContext:%d", resp.Seq)
	}
}

func (c *Client) AsyncCall(channel Channel, method string, arg interface{}, ret interface{}, deadline time.Time, callback func(interface{}, error)) error {
	b, err := c.codec.Encode(arg)
	if err != nil {
		return err
	}
	reqMessage := &RequestMsg{
		Seq:    c.makeSequence(),
		Method: method,
		Arg:    b,
		arg:    arg,
	}
	c.callOutInterceptor(reqMessage, arg)
	if ret == nil || callback == nil {
		reqMessage.Oneway = true
		return channel.Request(reqMessage)
	} else {
		ctx := asynCtxPool.Get().(*asynContext)
		ctx.fired = 0
		ctx.onResponse = callback
		ctx.ret = ret
		ctx.req = reqMessage
		ctx.cli = c
		c.pending(reqMessage.Seq).Store(reqMessage.Seq, ctx)
		ctx.timer = time.AfterFunc(time.Until(deadline), func() {
			if _, ok := c.pending(reqMessage.Seq).LoadAndDelete(reqMessage.Seq); ok {
				ctx.onTimeout()
			}
		})
		err = channel.Request(reqMessage)
		if err != nil {
			if _, ok := c.pending(reqMessage.Seq).LoadAndDelete(reqMessage.Seq); ok {
				if ctx.timer.Stop() {
					asynCtxPool.Put(ctx)
				}
			}
			if atomic.CompareAndSwapInt32(&ctx.fired, 0, 1) {
				c.callInInterceptor(reqMessage, nil, err)
			}
		}
		return err
	}
}

func rpcError(err error) *Error {
	switch err {
	case context.Canceled:
		return NewError(ErrCancel, "canceled")
	case context.DeadlineExceeded:
		return NewError(ErrTimeout, "timeout")
	default:
		return NewError(ErrOther, err.Error())
	}
}

func (c *Client) CallWithTimeout(channel Channel, method string, arg interface{}, ret interface{}, d time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return c.Call(ctx, channel, method, arg, ret)
}

func (c *Client) Call(ctx context.Context, channel Channel, method string, arg interface{}, ret interface{}) (err error) {
	b, err := c.codec.Encode(arg)
	if err != nil {
		return err
	}

	reqMessage := &RequestMsg{
		Seq:    c.makeSequence(),
		Method: method,
		Arg:    b,
		arg:    arg,
	}
	c.callOutInterceptor(reqMessage, arg)
	defer func() {
		c.callInInterceptor(reqMessage, ret, err)
	}()
	if ret == nil {
		reqMessage.Oneway = true
		for {
			if err = channel.RequestWithContext(ctx, reqMessage); err == nil {
				return nil
			} else if channel.IsRetryAbleError(err) {
				time.Sleep(time.Millisecond * 10)
				select {
				case <-ctx.Done():
					err = rpcError(ctx.Err())
					return err
				default:
					//context没有超时或被取消，继续尝试发送
				}
			} else {
				err = rpcError(err)
				return err
			}
		}
	} else {
		syncCtx := syncCtxPool.Get().(syncContext)
		c.pending(reqMessage.Seq).Store(reqMessage.Seq, syncCtx)
		for {
			if err = channel.RequestWithContext(ctx, reqMessage); err == nil {
				select {
				case resp := <-syncCtx:
					syncCtxPool.Put(syncCtx)
					if resp.Err != nil {
						err = resp.Err
						return err
					}
					if err = c.codec.Decode(resp.Ret, ret); err == nil {
						return err
					} else {
						err = rpcError(err)
						return err
					}
				case <-ctx.Done():
					if _, ok := c.pending(reqMessage.Seq).LoadAndDelete(reqMessage.Seq); ok {
						syncCtxPool.Put(syncCtx)
					}
					err = rpcError(ctx.Err())
					return err
				}
			} else if channel.IsRetryAbleError(err) {
				time.Sleep(time.Millisecond * 10)
				select {
				case <-ctx.Done():
					c.pending(reqMessage.Seq).Delete(reqMessage.Seq)
					syncCtxPool.Put(syncCtx)
					err = rpcError(ctx.Err())
					return err
				default:
					//context没有超时或被取消，继续尝试发送
				}
			} else {
				c.pending(reqMessage.Seq).Delete(reqMessage.Seq)
				syncCtxPool.Put(syncCtx)
				err = rpcError(err)
				return err
			}
		}
	}
}
```

- [ ] **Step 6: Write the failing test `rpc/rpc_test.go`**

```go
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
//   Request* -> server.OnMessage (async); Reply -> client.OnMessage.
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
func (c *loopChannel) Name() string                  { return c.name }
func (c *loopChannel) IsRetryAbleError(error) bool   { return false }

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
```

- [ ] **Step 7: Run the test**

Run: `cd /Users/huangwei/go_work/src/github.com/sniperHW/clustergo && go test ./rpc/ -run TestRPC -v`
Expected: PASS (all four `TestRPC_*`).

- [ ] **Step 8: Commit**

```bash
cd /Users/huangwei/go_work/src/github.com/sniperHW/clustergo
git add rpc/
git commit -m "refactor(rpc): inline simplified rpcgo into clustergo/rpc"
```

---

## Task 2: Create `clustergo/socket` package (TCP-only, poolbuff gather send)

**Files:**
- Create: `socket/socket.go`, `socket/socket_test.go`

- [ ] **Step 1: Create `socket/socket.go`**

```go
package socket

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sniperHW/netgo/poolbuff"
)

// MaxPacketSize bounds a single framed message: 4-byte length prefix + payload.
var MaxPacketSize = 1024 * 4

const headLen = 4

var (
	ErrSendQueueFull          = errors.New("send queue full")
	ErrPushToSendQueueTimeout = errors.New("push to send queue timeout")
	ErrSocketClosed           = errors.New("socket closed")
	ErrRecvTimeout            = errors.New("recv timeout")
)

// Codec encodes/decodes messages on a Socket.
type Codec interface {
	// Encode appends one fully-framed message (4-byte length prefix + payload) to dst,
	// returning the new slice and the number of bytes appended (0 means "encode nothing").
	Encode(dst []byte, o interface{}) (newDst []byte, n int)
	// Decode parses a payload into a message. The 4-byte length prefix is already
	// stripped by the Socket. The returned payload aliases an internal buffer and is
	// only valid until the next Recv; copy it if you must retain it past the handler.
	Decode(payload []byte) (interface{}, error)
}

type Options struct {
	SendChanSize    int
	BatchSendSize   int
	SendTimeout     time.Duration // async write deadline; 0 = no deadline
	AutoRecv        bool
	AutoRecvTimeout time.Duration
	Context         context.Context
}

type Socket struct {
	conn            *net.TCPConn
	codec           Codec
	die             chan struct{}
	sendReq         chan interface{}
	recvReq         chan time.Time
	closeOnce       sync.Once
	doCloseOnce     sync.Once
	closeReason     atomic.Value // error
	closeCallback   atomic.Value // func(error)
	packetHandler   atomic.Value // func(context.Context, interface{}) error
	onRecvTimeout   atomic.Value // func()
	autoRecv        bool
	autoRecvTimeout time.Duration
	batchSendSize   int
	sendTimeout     time.Duration
	context         context.Context
	sendDone        chan struct{}
	recvDone        chan struct{}
}

func New(conn *net.TCPConn, codec Codec, opts Options) *Socket {
	if opts.SendChanSize <= 0 {
		opts.SendChanSize = 1
	}
	if opts.BatchSendSize <= 0 {
		opts.BatchSendSize = 65535
	}
	if opts.Context == nil {
		opts.Context = context.Background()
	}
	s := &Socket{
		conn:            conn,
		codec:           codec,
		die:             make(chan struct{}),
		sendReq:         make(chan interface{}, opts.SendChanSize),
		recvReq:         make(chan time.Time, 1),
		autoRecv:        opts.AutoRecv,
		autoRecvTimeout: opts.AutoRecvTimeout,
		batchSendSize:   opts.BatchSendSize,
		sendTimeout:     opts.SendTimeout,
		context:         opts.Context,
		sendDone:        make(chan struct{}),
		recvDone:        make(chan struct{}),
	}
	s.closeCallback.Store(func(error) {})
	s.onRecvTimeout.Store(func() { s.close(ErrRecvTimeout) })
	s.packetHandler.Store(func(context.Context, interface{}) error { return nil })
	go s.sendloop()
	go s.recvloop()
	go s.finalize()
	return s
}

// finalize runs doClose once both loops have exited.
func (s *Socket) finalize() {
	<-s.sendDone
	<-s.recvDone
	s.doClose()
}

func (s *Socket) SetCloseCallback(cb func(error)) *Socket {
	if cb != nil {
		s.closeCallback.Store(cb)
	}
	return s
}

func (s *Socket) SetPacketHandler(h func(context.Context, interface{}) error) *Socket {
	if h != nil {
		s.packetHandler.Store(h)
	}
	return s
}

// ListenTCP wraps net.ListenTCP. Returns the listener, a serve function, and an error.
func ListenTCP(addr string, onConn func(*net.TCPConn)) (net.Listener, func(), error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, nil, err
	}
	ln, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		return nil, nil, err
	}
	serve := func() {
		for {
			conn, e := ln.Accept()
			if e == nil {
				onConn(conn.(*net.TCPConn))
			} else if ne, ok := e.(*net.OpError); ok && ne.Temporary() {
				time.Sleep(time.Millisecond * 10)
				continue
			} else {
				return
			}
		}
	}
	return ln, serve, nil
}

func (s *Socket) getTimeout(deadline time.Time) time.Duration {
	if deadline.IsZero() {
		return 0
	}
	return time.Until(deadline)
}

// deadline.IsZero(): when sendReq is full, wait forever.
// deadline in the past: when sendReq is full, return ErrSendQueueFull immediately.
// otherwise: when sendReq is full, wait until deadline then return ErrPushToSendQueueTimeout.
func (s *Socket) Send(o interface{}, deadline time.Time) error {
	if timeout := s.getTimeout(deadline); timeout == 0 {
		select {
		case <-s.die:
			return ErrSocketClosed
		case s.sendReq <- o:
			return nil
		}
	} else if timeout > 0 {
		t := time.NewTimer(timeout)
		defer t.Stop()
		select {
		case <-s.die:
			return ErrSocketClosed
		case <-t.C:
			return ErrPushToSendQueueTimeout
		case s.sendReq <- o:
			return nil
		}
	} else {
		select {
		case <-s.die:
			return ErrSocketClosed
		case s.sendReq <- o:
			return nil
		default:
			return ErrSendQueueFull
		}
	}
}

func (s *Socket) SendWithContext(ctx context.Context, o interface{}) error {
	select {
	case <-s.die:
		return ErrSocketClosed
	case s.sendReq <- o:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Recv arms one receive. With AutoRecv it keeps receiving after each packet.
func (s *Socket) Recv() {
	select {
	case <-s.die:
	case s.recvReq <- time.Time{}:
	default:
	}
}

func (s *Socket) Close(err error) {
	s.closeOnce.Do(func() {
		if err != nil {
			s.closeReason.Store(err)
		}
		close(s.die)
		s.conn.Close() // unblock any blocked read/write
	})
}

func (s *Socket) close(err error) {
	s.closeOnce.Do(func() {
		if err != nil {
			s.closeReason.Store(err)
		}
		close(s.die)
	})
}

func (s *Socket) doClose() {
	s.doCloseOnce.Do(func() {
		s.conn.Close()
		reason, _ := s.closeReason.Load().(error)
		s.closeCallback.Load().(func(error))(reason)
	})
}

// ---- send path (poolbuff gather) ----

func (s *Socket) write(buf []byte) error {
	deadline := time.Time{}
	if s.sendTimeout > 0 {
		deadline = time.Now().Add(s.sendTimeout)
	}
	if err := s.conn.SetWriteDeadline(deadline); err != nil {
		return err
	}
	_, err := s.conn.Write(buf)
	return err
}

func (s *Socket) sendloop() {
	defer close(s.sendDone)
	buf := poolbuff.Get()
	defer poolbuff.Put(buf)
	total := 0
	for {
		select {
		case <-s.die:
			// best-effort flush of anything already queued
			for len(s.sendReq) > 0 {
				o := <-s.sendReq
				before := len(buf)
				buf, _ = s.codec.Encode(buf, o)
				total += len(buf) - before
				if total >= s.batchSendSize {
					if s.write(buf) != nil {
						return
					}
					buf = buf[:0]
					total = 0
				}
			}
			if total > 0 {
				s.write(buf)
			}
			return
		case o := <-s.sendReq:
			before := len(buf)
			buf, _ = s.codec.Encode(buf, o)
			total += len(buf) - before
			if total >= s.batchSendSize || (total > 0 && len(s.sendReq) == 0) {
				if err := s.write(buf); err != nil {
					s.close(err)
					return
				}
				buf = buf[:0]
				total = 0
			}
		}
	}
}

// ---- recv path (length-prefix framing) ----

type frameReader struct {
	buf     []byte
	w, r    int
	maxSize int
}

// recv reads exactly one payload from conn. The returned slice aliases the
// internal buffer and is only valid until the next recv call.
func (fr *frameReader) recv(conn net.Conn, deadline time.Time) ([]byte, error) {
	for {
		unpackSize := fr.w - fr.r
		if unpackSize >= headLen {
			payload := int(binary.BigEndian.Uint32(fr.buf[fr.r:]))
			totalSize := payload + headLen
			if payload == 0 {
				return nil, fmt.Errorf("zero payload")
			} else if totalSize > fr.maxSize {
				return nil, fmt.Errorf("packet too large:%d", totalSize)
			} else if totalSize <= unpackSize {
				fr.r += headLen
				pkt := fr.buf[fr.r : fr.r+payload]
				fr.r += payload
				if fr.r == fr.w {
					fr.r = 0
					fr.w = 0
				}
				return pkt, nil
			} else {
				if totalSize > cap(fr.buf) {
					nb := make([]byte, totalSize)
					copy(nb, fr.buf[fr.r:fr.w])
					fr.buf = nb
				} else if fr.r > 0 {
					copy(fr.buf, fr.buf[fr.r:fr.w])
				}
				fr.w = fr.w - fr.r
				fr.r = 0
			}
		} else if fr.r > 0 {
			copy(fr.buf, fr.buf[fr.r:fr.w])
			fr.w = fr.w - fr.r
			fr.r = 0
		}
		if err := conn.SetReadDeadline(deadline); err != nil {
			return nil, err
		}
		n, err := conn.Read(fr.buf[fr.w:])
		if n > 0 {
			fr.w += n
		}
		if err != nil {
			return nil, err
		}
	}
}

func isNetTimeout(err error) bool {
	if e, ok := err.(net.Error); ok && e.Timeout() {
		return true
	}
	return false
}

func (s *Socket) recvloop() {
	defer close(s.recvDone)
	fr := &frameReader{buf: make([]byte, 4096), maxSize: MaxPacketSize}
	for {
		select {
		case <-s.die:
			return
		case deadline := <-s.recvReq:
			payload, err := fr.recv(s.conn, deadline)
			if err != nil {
				select {
				case <-s.die:
					return
				default:
				}
				if isNetTimeout(err) {
					s.onRecvTimeout.Load().(func())() // default closes the socket
				} else {
					s.close(err)
				}
				return
			}
			packet, derr := s.codec.Decode(payload)
			if derr != nil {
				s.close(derr)
				return
			}
			handler := s.packetHandler.Load().(func(context.Context, interface{}) error)
			if herr := handler(s.context, packet); herr != nil {
				s.close(herr)
				return
			}
			if s.autoRecv {
				var dl time.Time
				if s.autoRecvTimeout > 0 {
					dl = time.Now().Add(s.autoRecvTimeout)
				}
				select {
				case <-s.die:
				case s.recvReq <- dl:
				default:
				}
			}
		}
	}
}
```

- [ ] **Step 2: Write the failing test `socket/socket_test.go`**

```go
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
```

- [ ] **Step 3: Run the test**

Run: `cd /Users/huangwei/go_work/src/github.com/sniperHW/clustergo && go test ./socket/ -run TestSocket -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
cd /Users/huangwei/go_work/src/github.com/sniperHW/clustergo
git add socket/
git commit -m "refactor(socket): add TCP-only async socket with poolbuff gather send"
```

---

## Task 3: Switch `codec/ss` + root package to `rpc` + `socket` (atomic)

**Files:**
- Modify: `codec/ss/codec.go`, `codec/ss/message.go`, `node.go`, `rpc.go`, `cluster.go`, `logger.go`
- Modify: `cluster_test.go`, `codec/ss/ss_test.go`
- Modify: `example/pbrpc/genrpc/gen.go`, `example/pbrpc/service/echo/echo.go`, `example/pbrpc/service/test/test.go`
- Delete: `codec/receiver.go`

> **Note:** Intermediate states in this task do NOT compile, because the root package and its tests/examples switch API together. The commit at the end (Step 11) is the first green state.

- [ ] **Step 1: Rewrite `codec/ss/codec.go`**

Replace the entire file with:

```go
package ss

import (
	"encoding/binary"
	"fmt"
	"net"

	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/codec/buffer"
	"github.com/sniperHW/clustergo/codec/pb"
	"github.com/sniperHW/clustergo/rpc"
	"github.com/sniperHW/clustergo/socket"
	"google.golang.org/protobuf/proto"
)

const Namespace string = "ss"

var _ socket.Codec = (*SSCodec)(nil)

type SSCodec struct {
	selfAddr addr.LogicAddr
	reader   buffer.BufferReader
	pbMeta   *pb.PbMeta
}

func NewCodec(selfAddr addr.LogicAddr) *SSCodec {
	return &SSCodec{
		selfAddr: selfAddr,
		pbMeta:   pb.GetMeta(Namespace),
		reader:   buffer.NewReader(binary.BigEndian, nil),
	}
}

func (ss *SSCodec) encode(dst []byte, m *Message, cmd uint16, flag byte, data []byte) ([]byte, int) {
	payloadLen := sizeFlag + sizeToAndFrom + len(data)
	if flag == PbMsg || flag == BinMsg {
		payloadLen += sizeCmd
	}
	totalLen := sizeLen + payloadLen
	if totalLen > socket.MaxPacketSize {
		return dst, 0
	}
	w := buffer.NeWriter(binary.BigEndian)
	mark := len(dst)
	dst = append(dst, 0, 0, 0, 0) // length prefix placeholder
	binary.BigEndian.PutUint32(dst[mark:], uint32(payloadLen))
	dst = append(dst, flag)
	dst = w.AppendUint32(dst, uint32(m.To()))
	dst = w.AppendUint32(dst, uint32(m.From()))
	if flag == PbMsg || flag == BinMsg {
		dst = w.AppendUint16(dst, cmd)
	}
	dst = append(dst, data...)
	return dst, totalLen
}

func (ss *SSCodec) Encode(dst []byte, o interface{}) ([]byte, int) {
	switch o := o.(type) {
	case *Message:
		flag := byte(0)
		switch msg := o.Payload().(type) {
		case []byte:
			if len(msg) == 0 {
				return dst, 0
			}
			setMsgType(&flag, BinMsg)
			return ss.encode(dst, o, o.cmd, flag, msg)
		case proto.Message:
			data, cmd, err := ss.pbMeta.Marshal(msg)
			if err != nil {
				return dst, 0
			}
			setMsgType(&flag, PbMsg)
			return ss.encode(dst, o, uint16(cmd), flag, data)
		case *rpc.RequestMsg:
			setMsgType(&flag, RpcReq)
			return ss.encode(dst, o, 0, flag, rpc.EncodeRequest(msg))
		case *rpc.ResponseMsg:
			setMsgType(&flag, RpcResp)
			return ss.encode(dst, o, 0, flag, rpc.EncodeResponse(msg))
		}
		return dst, 0
	case *RelayMessage:
		return append(dst, o.Payload()...), len(o.Payload())
	default:
		return dst, 0
	}
}

func (ss *SSCodec) isTarget(to addr.LogicAddr) bool {
	return ss.selfAddr == to
}

func (ss *SSCodec) Decode(payload []byte) (interface{}, error) {
	ss.reader.Reset(payload)
	flag := ss.reader.GetByte()
	to := addr.LogicAddr(ss.reader.GetUint32())
	from := addr.LogicAddr(ss.reader.GetUint32())
	if ss.isTarget(to) {
		switch getMsgType(flag) {
		case BinMsg:
			cmd := ss.reader.GetUint16()
			data := ss.reader.GetAll()
			return NewMessage(to, from, data, cmd), nil
		case PbMsg:
			cmd := ss.reader.GetUint16()
			data := ss.reader.GetAll()
			if msg, err := ss.pbMeta.Unmarshal(uint32(cmd), data); err != nil {
				return nil, err
			} else {
				return NewMessage(to, from, msg, cmd), nil
			}
		case RpcReq:
			if req, err := rpc.DecodeRequest(ss.reader.GetAll()); err != nil {
				return nil, err
			} else {
				return NewMessage(to, from, req), nil
			}
		case RpcResp:
			if resp, err := rpc.DecodeResponse(ss.reader.GetAll()); err != nil {
				return nil, err
			} else {
				return NewMessage(to, from, resp), nil
			}
		default:
			return nil, fmt.Errorf("invaild packet type")
		}
	} else {
		return NewRelayMessage(to, from, payload), nil
	}
}
```

> The struct above is named `SSCodec` (consistent with `NewCodec(...) *SSCodec`, the `*SSCodec` receivers, and `var _ socket.Codec = (*SSCodec)(nil)`). It replaces the old `SSCodec` which embedded `codec.LengthPayloadPacketReceiver` — that embedding is gone; framing now lives in the `socket` package.

- [ ] **Step 2: Update `codec/ss/message.go`**

Two edits to `/Users/huangwei/go_work/src/github.com/sniperHW/clustergo/codec/ss/message.go`:

1. Change the import block: replace `"github.com/sniperHW/rpcgo"` with `"github.com/sniperHW/clustergo/rpc"`.
2. Change every `rpcgo.` reference in the file to `rpc.`:
   - `func (m *RelayMessage) GetRpcRequest() *rpcgo.RequestMsg` → `*rpc.RequestMsg`
   - inside it: `rpcgo.DecodeRequest(...)` → `rpc.DecodeRequest(...)`
3. **Delete** the `MaxPacketSize` variable (it now lives in `socket`). Remove these two lines:
```go
var (
	MaxPacketSize = 1024 * 4
)
```
   Keep all other consts (`sizeLen`, `sizeFlag`, `sizeToAndFrom`, `sizeCmd`, `sizeRpcSeqNo`, `minSize`, and the message-type `PbMsg`/`BinMsg`/`RpcReq`/`RpcResp`/`MaskMessageType`).

- [ ] **Step 3: Delete `codec/receiver.go`**

```bash
cd /Users/huangwei/go_work/src/github.com/sniperHW/clustergo
git rm codec/receiver.go
```

- [ ] **Step 4: Rewire `node.go`**

Apply these edits to `/Users/huangwei/go_work/src/github.com/sniperHW/clustergo/node.go`:

1. **Imports** — replace:
```go
	"github.com/sniperHW/netgo"
	"github.com/sniperHW/rpcgo"
```
with:
```go
	"github.com/sniperHW/clustergo/rpc"
	"github.com/sniperHW/clustergo/socket"
```

2. **`node` struct field** — in the `type node struct {...}` block, change:
```go
	socket     *netgo.AsynSocket
```
to:
```go
	socket     *socket.Socket
```

3. **`onEstablish`** — replace the function body's socket setup. Change:
```go
	codec := ss.NewCodec(self.localAddr.LogicAddr())
	n.socket = netgo.NewAsynSocket(netgo.NewTcpSocket(conn, codec), netgo.AsynSocketOption{
		SendChanSize: SendChanSize,
		Codec:        codec,
		AutoRecv:     true,
		Context:      context.TODO(),
	})

	n.socket.SetPacketHandler(func(ctx context.Context, as *netgo.AsynSocket, packet interface{}) error {
		if self.getNodeByLogicAddr(n.addr.LogicAddr()) != n {
			return ErrInvaildNode
		} else {
			n.onMessage(ctx, self, packet)
			return nil
		}
	})

	n.socket.SetCloseCallback(func(as *netgo.AsynSocket, err error) {
		n.Lock()
		n.socket = nil
		n.Unlock()
	}).Recv()
```
to:
```go
	codec := ss.NewCodec(self.localAddr.LogicAddr())
	n.socket = socket.New(conn, codec, socket.Options{
		SendChanSize:  SendChanSize,
		BatchSendSize: BatchSendSize,
		AutoRecv:      true,
		Context:       context.TODO(),
	})

	n.socket.SetPacketHandler(func(ctx context.Context, packet interface{}) error {
		if self.getNodeByLogicAddr(n.addr.LogicAddr()) != n {
			return ErrInvaildNode
		} else {
			n.onMessage(ctx, self, packet)
			return nil
		}
	})

	n.socket.SetCloseCallback(func(err error) {
		n.Lock()
		n.socket = nil
		n.Unlock()
	}).Recv()
```

4. **`onRelayMessage`** — change every `rpcgo.` to `rpc.`:
   - `*rpcgo.ResponseMsg` → `*rpc.ResponseMsg`
   - `rpcgo.NewError(rpcgo.ErrOther, ...)` → `rpc.NewError(rpc.ErrOther, ...)`

5. **`onMessage`** — change `*rpcgo.RequestMsg` → `*rpc.RequestMsg`, `*rpcgo.ResponseMsg` → `*rpc.ResponseMsg`, and change the response-delivery call:
```go
		case *rpcgo.ResponseMsg:
			self.rpcCli.cli.OnMessage(nil, m)
```
to:
```go
		case *rpc.ResponseMsg:
			self.rpcCli.cli.OnMessage(m)
```

- [ ] **Step 5: Rewire `rpc.go`**

Apply these edits to `/Users/huangwei/go_work/src/github.com/sniperHW/clustergo/rpc.go`:

1. **Imports** — replace:
```go
	"github.com/sniperHW/netgo"
	"github.com/sniperHW/rpcgo"
```
with:
```go
	"github.com/sniperHW/clustergo/rpc"
	"github.com/sniperHW/clustergo/socket"
```

2. Replace **every** `rpcgo.` in the file with `rpc.` (RequestMsg, ResponseMsg, Replyer, Channel, Codec, Server, Client, NewClient, NewServer, Register, NewError, ErrOther). E.g. `func (c *rpcChannel) RequestWithContext(ctx context.Context, request *rpcgo.RequestMsg) error` → `*rpc.RequestMsg`.

3. **Delete** the `Identity()` method from `rpcChannel` (it is unused):
```go
func (c *rpcChannel) Identity() uint64 {
	return *(*uint64)(unsafe.Pointer(c.node))
}
```
After deleting it, also remove the now-unused import `"unsafe"` from the import block.

4. **`rpcChannel.IsRetryAbleError`** — change:
```go
	switch err {
	case ErrPendingQueueFull, netgo.ErrSendQueueFull, netgo.ErrPushToSendQueueTimeout:
		return true
```
to:
```go
	switch err {
	case ErrPendingQueueFull, socket.ErrSendQueueFull, socket.ErrPushToSendQueueTimeout:
		return true
```

5. **`selfChannel.Reply`** — change:
```go
		c.self.rpcCli.cli.OnMessage(nil, response)
```
to:
```go
		c.self.rpcCli.cli.OnMessage(response)
```

6. **`RPCClient.AsyncCall`** — change:
```go
	case ErrPendingQueueFull, netgo.ErrSendQueueFull:
		return ErrBusy
```
to:
```go
	case ErrPendingQueueFull, socket.ErrSendQueueFull:
		return ErrBusy
```

- [ ] **Step 6: Rewire `cluster.go`**

Apply these edits to `/Users/huangwei/go_work/src/github.com/sniperHW/clustergo/cluster.go`:

1. **Imports** — replace:
```go
	"github.com/sniperHW/netgo"
	"github.com/sniperHW/rpcgo"
```
with:
```go
	"github.com/sniperHW/clustergo/rpc"
	"github.com/sniperHW/clustergo/socket"
```

2. **Config var** — add `BatchSendSize` next to the existing config vars (after `MaxPendingMsgSize`):
```go
var (
	SendChanSize       int           = 256
	DefaultSendTimeout time.Duration = time.Millisecond * 200
	MaxPendingMsgSize  int           = 1024
	BatchSendSize      int           = 65535
)
```

3. **`RPCCodec`** — change `var RPCCodec rpcgo.Codec = PbCodec{}` to `var RPCCodec rpc.Codec = PbCodec{}`.

4. **`Start` listener** — change:
```go
		s.listener, serve, err = netgo.ListenTCP("tcp", s.localAddr.NetAddr().String(), func(conn *net.TCPConn) {
```
to:
```go
		s.listener, serve, err = socket.ListenTCP(s.localAddr.NetAddr().String(), func(conn *net.TCPConn) {
```

5. **`NewClusterNode`** — change the signature and the rpc construction:
```go
func NewClusterNode(rpccodec rpcgo.Codec) *Node {
```
to:
```go
func NewClusterNode(rpccodec rpc.Codec) *Node {
```
and change:
```go
		cli: rpcgo.NewClient(rpccodec),
```
to:
```go
		cli: rpc.NewClient(rpccodec),
```
and:
```go
		svr: rpcgo.NewServer(rpccodec),
```
to:
```go
		svr: rpc.NewServer(rpccodec),
```
and:
```go
	n.rpcSvr.SetInInterceptor([]func(*rpcgo.Replyer, *rpcgo.RequestMsg) bool{})
```
to:
```go
	n.rpcSvr.SetInInterceptor([]func(*rpc.Replyer, *rpc.RequestMsg) bool{})
```

6. **`RPCServer.SetInInterceptor`** receiver/method — replace `*rpcgo.Replyer` → `*rpc.Replyer`, `*rpcgo.RequestMsg` → `*rpc.RequestMsg` throughout the method:
```go
func (s *RPCServer) SetInInterceptor(interceptor []func(*rpc.Replyer, *rpc.RequestMsg) bool) {
	s.svr.SetInInterceptor(append(interceptor, func(replyer *rpc.Replyer, req *rpc.RequestMsg) bool {
		s.pendingRespCount.Add(1)
		replyer.AppendOutInterceptor(func(req *rpc.RequestMsg, ret interface{}, err error) {
			s.pendingRespCount.Add(-1)
		})
		return true
	}))
}
```
Also change the `RPCServer` struct field `svr *rpcgo.Server` → `svr *rpc.Server`.

7. **`RPCClient`** — change struct field `cli *rpcgo.Client` → `cli *rpc.Client`, and update `SetInInterceptor`/`SetOutInterceptor` signatures:
```go
func (c *RPCClient) SetInInterceptor(interceptor []func(*rpc.RequestMsg, interface{}, error)) {
	c.cli.SetInInterceptor(interceptor)
}

func (c *RPCClient) SetOutInterceptor(interceptor []func(*rpc.RequestMsg, interface{})) {
	c.cli.SetOutInterceptor(interceptor)
}
```
In `RPCClient.AsyncCall` and `RPCClient.Call`/`CallWithTimeout`, change `rpcgo.NewError(rpcgo.ErrOther, ...)` → `rpc.NewError(rpc.ErrOther, ...)`.

8. **Generic helpers** — change the four public helpers and `registerService` to use `rpc`:
```go
func RegisterService[Arg any](name string, method func(context.Context, *rpc.Replyer, *Arg)) error {
	return rpc.Register(GetDefaultNode().rpcSvr.svr, name, method)
}
```
and `registerService`:
```go
func registerService[Arg any](node *Node, name string, method func(context.Context, *rpc.Replyer, *Arg)) error {
	return rpc.Register(node.rpcSvr.svr, name, method)
}
```

- [ ] **Step 7: Rewire `logger.go`**

Replace the entire content of `/Users/huangwei/go_work/src/github.com/sniperHW/clustergo/logger.go` with:

```go
package clustergo

import (
	"github.com/sniperHW/clustergo/rpc"
)

type Logger interface {
	Debugf(string, ...interface{})
	Infof(string, ...interface{})
	Warnf(string, ...interface{})
	Errorf(string, ...interface{})
	Panicf(string, ...interface{})
	Fatalf(string, ...interface{})
	Debug(...interface{})
	Info(...interface{})
	Warn(...interface{})
	Error(...interface{})
	Panic(...interface{})
	Fatal(...interface{})
}

var logger Logger

func InitLogger(l Logger) {
	rpc.InitLogger(l)
	logger = l
}

func Log() Logger {
	return logger
}
```

- [ ] **Step 8: Rewrite `codec/ss/ss_test.go`**

The old test used the `net.Buffers`-based `Encode(buffs, msg)` signature and called `codec.Recv` (the embedded `LengthPayloadPacketReceiver`, now removed). Replace the **entire file** `/Users/huangwei/go_work/src/github.com/sniperHW/clustergo/codec/ss/ss_test.go` with:

```go
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
```

(Run later in Step 11: `go test ./codec/ss/ -v` — all four tests should PASS.)

- [ ] **Step 9: Update `cluster_test.go`**

In `/Users/huangwei/go_work/src/github.com/sniperHW/clustergo/cluster_test.go`:
1. Replace import `"github.com/sniperHW/rpcgo"` with `"github.com/sniperHW/clustergo/rpc"`.
2. Replace every `rpcgo.` in the file with `rpc.` (covers `rpcgo.Replyer`, `rpcgo.RequestMsg`, `rpcgo.Register`, `rpcgo.NewError`, `rpcgo.ErrOther`, `rpcgo.ResponseMsg`).
   - In `registerService`: `func(context.Context, *rpcgo.Replyer, *string)` → `*rpc.Replyer`; `rpcgo.Register(...)` → `rpc.Register(...)`.
   - In the interceptor setup: `[]func(replyer *rpcgo.Replyer, req *rpcgo.RequestMsg) bool` → `*rpc.Replyer`/`*rpc.RequestMsg`; `replyer.AppendOutInterceptor(func(req *rpcgo.RequestMsg, ...) {...})` → `*rpc.RequestMsg`.

- [ ] **Step 10: Update `example/pbrpc`**

These three files use `rpcgo` and clustergo's RPC API:

1. `example/pbrpc/service/echo/echo.go` — replace import `"github.com/sniperHW/rpcgo"` with `"github.com/sniperHW/clustergo/rpc"`; change `replyer *rpcgo.Replyer` → `*rpc.Replyer`; change `func (r *Replyer) Channel() rpcgo.Channel` → `rpc.Channel`.
2. `example/pbrpc/service/test/test.go` — same edits as echo.go.
3. `example/pbrpc/genrpc/gen.go` — replace import `"github.com/sniperHW/rpcgo"` with `"github.com/sniperHW/clustergo/rpc"`; in the template strings change `rpcgo.Replyer` → `rpc.Replyer` and `rpcgo.Channel` → `rpc.Channel`; change the non-template `replyer *rpcgo.Replyer` → `*rpc.Replyer` and `func (r *Replyer) Channel() rpcgo.Channel` → `rpc.Channel`.

- [ ] **Step 11: Build and test (first green state)**

Run:
```bash
cd /Users/huangwei/go_work/src/github.com/sniperHW/clustergo
go build ./...
```
Expected: builds with no errors. If `example/stream` or `example/membership` fail to build, re-check that no change touched their clustergo API usage (they must remain untouched).

Run:
```bash
go test ./...
```
Expected: all packages PASS (`rpc`, `socket`, `codec/ss`, and the root `clustergo` package's `cluster_test.go`).

- [ ] **Step 12: Commit**

```bash
cd /Users/huangwei/go_work/src/github.com/sniperHW/clustergo
git add -A
git commit -m "refactor: switch clustergo from rpcgo/netgo.AsynSocket to rpc/socket"
```

---

## Task 4: Drop `rpcgo` from `go.mod` and final validation

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Tidy modules**

```bash
cd /Users/huangwei/go_work/src/github.com/sniperHW/clustergo
go mod tidy
```
Expected: `go.mod` no longer lists `github.com/sniperHW/rpcgo`; `github.com/sniperHW/netgo` remains (poolbuff). `go.sum` updated.

- [ ] **Step 2: Verify no stray rpcgo / AsynSocket references**

Run two checks. First, confirm `rpcgo` is gone everywhere:
```bash
cd /Users/huangwei/go_work/src/github.com/sniperHW/clustergo
grep -rn 'sniperHW/rpcgo' --include='*.go' . | grep -v vendor
```
Expected: no matches.

Second, confirm no **library** code (root package + `rpc/`, `socket/`, `codec/`, `addr/`, `membership/`, `pkg/`) references the removed netgo symbols. `example/stream` and `example/membership` legitimately use `netgo` directly (out of scope), so exclude them:
```bash
grep -rn 'netgo\.AsynSocket\|netgo\.NewAsynSocket\|netgo\.NewTcpSocket\|netgo\.ListenTCP\|netgo\.ReadAble' --include='*.go' . | grep -v vendor | grep -vE 'example/(stream|membership)/'
```
Expected: no matches.

- [ ] **Step 3: Full build + vet + test**

Run:
```bash
cd /Users/huangwei/go_work/src/github.com/sniperHW/clustergo
go build ./...
go vet ./...
go test ./... -race
```
Expected: all pass with `-race` clean.

- [ ] **Step 4: Commit**

```bash
cd /Users/huangwei/go_work/src/github.com/sniperHW/clustergo
git add go.mod go.sum
git commit -m "chore: drop rpcgo dependency (inlined into clustergo/rpc)"
```

---

## Done criteria

- `rpcgo` no longer in `go.mod`; `netgo` retained for `poolbuff` only.
- No `netgo.AsynSocket` / `netgo.NewTcpSocket` / `netgo.ListenTCP` / `netgo.ReadAble` references outside `example/stream` and `example/membership` (which use netgo directly and are out of scope).
- `go build ./...`, `go vet ./...`, `go test ./... -race` all green.
- Send path uses `poolbuff` gather buffer (`socket.sendloop`), single `conn.Write` per batch.
