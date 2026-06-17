package rpc

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
)

type method func(context.Context, *Server, *RequestMsg, *Replyer) error

type Replyer struct {
	channel        Channel
	replyed        int32
	req            *RequestMsg
	outInterceptor []func(*RequestMsg, interface{}, error)
}

func (r *Replyer) AppendOutInterceptor(interceptor func(*RequestMsg, interface{}, error)) {
	r.outInterceptor = append(r.outInterceptor, interceptor)
}

func (r *Replyer) callOutInterceptor(ret interface{}, err error) {
	for _, fn := range r.outInterceptor {
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("%v: %s", r, debug.Stack())
				}
			}()
			fn(r.req, ret, err)
		}()
	}
}

func (r *Replyer) Error(err error) {
	if atomic.CompareAndSwapInt32(&r.replyed, 0, 1) {
		r.callOutInterceptor(nil, err)
		if r.req.Oneway {
			return
		}
		resp := &ResponseMsg{
			Seq: r.req.Seq,
		}

		if _, ok := err.(*Error); ok {
			resp.Err = err.(*Error)
		} else {
			resp.Err = NewError(ErrMethod, err.Error())
		}

		if e := r.channel.Reply(resp); e != nil {
			logger.Errorf("send rpc response to (%s) error:%s\n", r.channel.Name(), e.Error())
		}
	}
}

func (r *Replyer) Reply(ret interface{}) {
	if atomic.CompareAndSwapInt32(&r.replyed, 0, 1) {
		r.callOutInterceptor(ret, nil)
		if r.req.Oneway {
			return
		}
		resp := &ResponseMsg{
			Seq: r.req.Seq,
		}
		resp.Ret = ret

		if e := r.channel.Reply(resp); e != nil {
			logger.Errorf("send rpc response to (%s) error:%s\n", r.channel.Name(), e.Error())
		}
	}
}

func (r *Replyer) Channel() Channel {
	return r.channel
}

type Server struct {
	sync.RWMutex
	methods       map[string]method
	stoped        atomic.Bool
	inInterceptor []func(*Replyer, *RequestMsg) bool //入站管道线
}

func NewServer() *Server {
	return &Server{
		methods: map[string]method{},
	}
}

func (s *Server) SetInInterceptor(interceptor []func(*Replyer, *RequestMsg) bool) *Server {
	s.inInterceptor = interceptor
	return s
}

func (s *Server) Stop() {
	s.stoped.CompareAndSwap(false, true)
}

func (s *Server) method(name string) method {
	s.RLock()
	defer s.RUnlock()
	return s.methods[name]
}

func (s *Server) OnMessage(context context.Context, channel Channel, req *RequestMsg) {
	replyer := &Replyer{channel: channel, req: req}
	if s.stoped.Load() {
		replyer.Error(NewError(ErrServiceUnavaliable, "service unavaliable"))
		return
	}
	fn := s.method(req.Method)
	if fn == nil {
		replyer.Error(NewError(ErrInvaildMethod, fmt.Sprintf("method %s not found", req.Method)))
		return
	}

	if err := fn(context, s, req, replyer); err != nil {
		replyer.Error(err)
	}
}
