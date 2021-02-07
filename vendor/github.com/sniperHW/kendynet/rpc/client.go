package rpc

import (
	"container/list"
	"fmt"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/timer"
	"github.com/sniperHW/kendynet/util"
	"sync"
	"sync/atomic"
	"time"
)

var ErrCallTimeout error = fmt.Errorf("rpc call timeout")
var ErrChannelDisconnected error = fmt.Errorf("channel disconnected")
var sequence uint64
var client_once sync.Once
var timerMgrs []*timer.TimerMgr

type RPCResponseHandler func(interface{}, error)

type reqContext struct {
	seq        uint64
	onResponse RPCResponseHandler
	listEle    *list.Element
	channelUID uint64
	c          *RPCClient
}

type channelReqContexts struct {
	reqs *list.List
}

func (this *channelReqContexts) add(req *reqContext) {
	req.listEle = this.reqs.PushBack(req)
}

func (this *channelReqContexts) remove(req *reqContext) bool {
	if nil != req.listEle {
		this.reqs.Remove(req.listEle)
		req.listEle = nil
		return true
	} else {
		return false
	}
}

func (this *reqContext) onTimeout(_ *timer.Timer, _ interface{}) {
	kendynet.GetLogger().Infoln("req timeout", this.seq, time.Now())
	if this.c.removeChannelReq(this) {
		this.onResponse(nil, ErrCallTimeout)
	}
}

type channelReqMap struct {
	sync.Mutex
	m map[uint64]*channelReqContexts
}

type RPCClient struct {
	encoder        RPCMessageEncoder
	decoder        RPCMessageDecoder
	channelReqMaps []channelReqMap
}

func (this *RPCClient) addChannelReq(channel RPCChannel, req *reqContext) {
	uid := channel.UID()
	m := this.channelReqMaps[int(uid)%len(this.channelReqMaps)]

	m.Lock()
	defer m.Unlock()
	c, ok := m.m[uid]
	if !ok {
		c = &channelReqContexts{
			reqs: list.New(),
		}
		m.m[uid] = c
	}
	c.add(req)
}

func (this *RPCClient) removeChannelReq(req *reqContext) bool {
	uid := req.channelUID
	m := this.channelReqMaps[int(uid)%len(this.channelReqMaps)]

	m.Lock()
	defer m.Unlock()

	c, ok := m.m[uid]
	if ok {
		ret := c.remove(req)
		if c.reqs.Len() == 0 {
			delete(m.m, uid)
		}
		return ret
	} else {
		return false
	}
}

func (this *RPCClient) OnChannelDisconnect(channel RPCChannel) {
	uid := channel.UID()
	m := this.channelReqMaps[int(uid)%len(this.channelReqMaps)]

	var tmp []*reqContext

	m.Lock()
	c, ok := m.m[uid]
	if ok {
		delete(m.m, uid)
		tmp = make([]*reqContext, 0, c.reqs.Len())
		for {
			if v := c.reqs.Front(); nil != v {
				v.Value.(*reqContext).listEle = nil
				tmp = append(tmp, v.Value.(*reqContext))
				c.reqs.Remove(v)
			} else {
				break
			}
		}
	}
	m.Unlock()

	for _, v := range tmp {
		/*
		 * 不管CancelByIndex是否返回true都应该调用onResponse
		 * 考虑如下onResponse调用丢失的场景
		 * A线程执行OnChannelDisconnect,走到m.Unlock()之后的代码
		 * B线程执行onTimeout的removeChannelReq,此时removeChannelReq必然返回false,因此onResponse不会执行。
		 * A线程继续执行,如果像OnRPCMessage一样判断CancelByIndex为true才执行onResponse,那么对于这个请求的onResponse将丢失
		 * 因为这个req的onTimeout已经被执行，CancelByIndex必定返回false
		 */
		timerMgrs[v.seq%uint64(len(timerMgrs))].CancelByIndex(v.seq)
		v.onResponse(nil, ErrChannelDisconnected)
	}
}

//收到RPC消息后调用
func (this *RPCClient) OnRPCMessage(message interface{}) {
	if msg, err := this.decoder.Decode(message); nil != err {
		kendynet.GetLogger().Errorf(util.FormatFileLine("RPCClient rpc message decode err:%s\n", err.Error()))
	} else {
		if resp, ok := msg.(*RPCResponse); ok {
			mgr := timerMgrs[msg.GetSeq()%uint64(len(timerMgrs))]
			if ok, ctx := mgr.CancelByIndex(resp.GetSeq()); ok {
				if this.removeChannelReq(ctx.(*reqContext)) {
					ctx.(*reqContext).onResponse(resp.Ret, resp.Err)
				}
			} else if nil == ctx {
				kendynet.GetLogger().Infoln("onResponse with no reqContext", resp.GetSeq())
			}
		}
	}
}

//投递，不关心响应和是否失败
func (this *RPCClient) Post(channel RPCChannel, method string, arg interface{}) error {

	req := &RPCRequest{
		Method:   method,
		Seq:      atomic.AddUint64(&sequence, 1),
		Arg:      arg,
		NeedResp: false,
	}

	if request, err := this.encoder.Encode(req); nil != err {
		return fmt.Errorf("encode error:%s\n", err.Error())
	} else {
		if err = channel.SendRequest(request); nil != err {
			return err
		} else {
			return nil
		}
	}
}

func (this *RPCClient) AsynCall(channel RPCChannel, method string, arg interface{}, timeout time.Duration, cb RPCResponseHandler) error {

	if cb == nil {
		panic("cb == nil")
	}

	req := &RPCRequest{
		Method:   method,
		Seq:      atomic.AddUint64(&sequence, 1),
		Arg:      arg,
		NeedResp: true,
	}

	context := &reqContext{
		onResponse: cb,
		seq:        req.Seq,
		c:          this,
		channelUID: channel.UID(),
	}

	if request, err := this.encoder.Encode(req); err != nil {
		return err
	} else {
		mgr := timerMgrs[req.Seq%uint64(len(timerMgrs))]
		this.addChannelReq(channel, context)
		mgr.OnceWithIndex(timeout, context.onTimeout, context, context.seq)
		if err = channel.SendRequest(request); err == nil {
			return nil
		} else {
			this.removeChannelReq(context)
			mgr.CancelByIndex(context.seq)
			return err
		}
	}
}

//同步调用
func (this *RPCClient) Call(channel RPCChannel, method string, arg interface{}, timeout time.Duration) (ret interface{}, err error) {
	waitC := make(chan struct{})
	f := func(ret_ interface{}, err_ error) {
		ret = ret_
		err = err_
		close(waitC)
	}

	if err = this.AsynCall(channel, method, arg, timeout, f); nil == err {
		<-waitC
	}

	return
}

func NewClient(decoder RPCMessageDecoder, encoder RPCMessageEncoder) *RPCClient {
	if nil == decoder || nil == encoder {
		return nil
	} else {

		client_once.Do(func() {
			timerMgrs = make([]*timer.TimerMgr, 61)
			for i, _ := range timerMgrs {
				timerMgrs[i] = timer.NewTimerMgr(1)
			}
		})

		c := &RPCClient{
			encoder:        encoder,
			decoder:        decoder,
			channelReqMaps: make([]channelReqMap, 127, 127),
		}

		for k, _ := range c.channelReqMaps {
			c.channelReqMaps[k] = channelReqMap{
				m: map[uint64]*channelReqContexts{},
			}
		}

		return c
	}
}
