package rpc

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"

	"github.com/sniperHW/clustergo/logger"
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

// Register 注册一个RPC方法。name为方法名，method为业务处理函数；Arg为请求参数类型，
// 框架在调用method之前会把请求参数解码成*Arg。Arg通常由method的函数类型推导得出，
// 如: s.Register("echo", func(ctx context.Context, r *Replyer, arg *EchoArg) { ... })。
func (s *Server) Register[Arg any](name string, method func(context.Context, *Replyer, *Arg)) error {
	s.Lock()
	defer s.Unlock()
	if name == "" {
		return errors.New("RegisterMethod nams is nil")
	} else if _, ok := s.methods[name]; ok {
		return fmt.Errorf("duplicate method:%s", name)
	} else {

		caller := func(c context.Context, s *Server, req *RequestMsg, replyer *Replyer) (err error) {
			arg := new(Arg)
			if err = req.decodeArgInto(arg); err != nil {
				return NewError(ErrOther, fmt.Sprintf("arg decode error:%v", err))
			}
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("method:%s channel:%s %s", req.Method, replyer.channel.Name(), fmt.Sprintf("%v: %s", r, string(debug.Stack())))
					err = NewError(ErrOther, "method panic")
				}
			}()
			req.Arg = arg

			for _, v := range s.inInterceptor {
				if !v(replyer, req) {
					return
				}
			}
			method(c, replyer, arg)
			return nil
		}
		s.methods[name] = caller
		return nil
	}
}

// UnRegister 移除已注册的RPC方法。
func (s *Server) UnRegister(name string) {
	s.Lock()
	defer s.Unlock()
	delete(s.methods, name)
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
