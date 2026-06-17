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

// New creates a Socket over conn and eagerly starts its send, recv, and finalize
// goroutines. The loops remain idle (blocked on their selects) until Send/Recv is
// first called, so callbacks (SetCloseCallback/SetPacketHandler) must be set before
// the first Send/Recv. The caller must eventually call Close to release the conn
// and goroutines.
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
	s.close(err)
}

func (s *Socket) close(err error) {
	s.closeOnce.Do(func() {
		if err != nil {
			s.closeReason.Store(err)
		}
		close(s.die)
		// Close the conn so a sibling loop blocked in Read/Write returns and finalize can complete.
		s.conn.Close()
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
	defer func() { poolbuff.Put(buf) }()
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
