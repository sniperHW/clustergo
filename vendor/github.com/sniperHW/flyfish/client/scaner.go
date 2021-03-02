package client

/*
import (
	"fmt"
	"github.com/sniperHW/flyfish/errcode"
	protocol "github.com/sniperHW/flyfish/proto"
	"sync/atomic"

	//"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet/event"
	"github.com/sniperHW/kendynet/util"
)

type Scaner struct {
	conn          *Conn
	callbackQueue *event.EventQueue //响应回调的事件队列
	table         string
	fileds        []string
	getAll        bool //获取所有字段
	first         int32
	closed        int32
}

//如果不传fields表示getAll
func (this *Client) Scaner(table string, fileds ...string) *Scaner {
	s := &Scaner{
		callbackQueue: this.callbackQueue,
		table:         table,
		fileds:        fileds,
	}
	if len(fileds) == 0 {
		s.getAll = true
	}
	s.conn = openConn(this, this.service)
	return s
}

func (this *Scaner) wrapCb(cb func(*Scaner, *MutiResult)) func(*MutiResult) {
	return func(r *MutiResult) {
		defer util.Recover(logger)
		cb(this, r)
	}
}

func (this *Scaner) AsyncNext(count int32, cb func(*Scaner, *MutiResult)) error {

	if atomic.LoadInt32(&this.closed) == 1 {
		return fmt.Errorf("closed")
	}

	if count <= 0 {
		count = 50
	}

	req := &protocol.ScanReq{
		Head: &protocol.ReqCommon{
			Seqno:       atomic.AddInt64(&this.conn.seqno, 1), //proto.Int64(atomic.AddInt64(&this.conn.seqno, 1)),
			Timeout:     int64(ServerTimeout),                 //proto.Int64(int64(ServerTimeout)),
			RespTimeout: int64(ClientTimeout),                 //proto.Int64(int64(ClientTimeout)),
		},
	}
	if atomic.CompareAndSwapInt32(&this.first, 0, 1) {
		req.Head.Table = this.table //proto.String(this.table)
		if this.getAll {
			req.All = true //proto.Bool(true)
		} else {
			req.All = false //proto.Bool(false)
			req.Fields = []string{}
			for _, v := range this.fileds {
				req.Fields = append(req.Fields, v)
			}
			if len(req.Fields) == 0 {
				req.All = true //proto.Bool(true)
			}
		}
	}
	req.Count = count //proto.Int32(count)

	context := &cmdContext{
		seqno: req.Head.GetSeqno(),
		cb: callback{
			tt: cb_muti,
			cb: this.wrapCb(cb),
		},
		req: req,
	}
	this.conn.exec(context)

	return nil
}

func (this *Scaner) Next(count int32) (*MutiResult, error) {
	respChan := make(chan *MutiResult)
	err := this.AsyncNext(count, func(s *Scaner, r *MutiResult) {
		respChan <- r
	})
	if nil != err {
		return nil, err
	}
	return <-respChan, nil
}

func (this *Scaner) Close() {
	if atomic.CompareAndSwapInt32(&this.closed, 0, 1) {
		this.conn.Close()
	}
}

func (this *Conn) onScanResp(resp *protocol.ScanResp) {
	c := this.removeContext(resp.Head.GetSeqno())
	if nil != c {
		ret := MutiResult{
			ErrCode: resp.Head.GetErrCode(),
			Rows:    []*Row{},
		}

		if ret.ErrCode == errcode.ERR_OK {
			for _, v := range resp.GetRows() {
				fields := v.GetFields()
				r := &Row{
					Key:     v.GetKey(),
					Version: v.GetVersion(),
					Fields:  map[string]*Field{},
				}

				for _, field := range fields {
					r.Fields[field.GetName()] = (*Field)(field)
				}

				ret.Rows = append(ret.Rows, r)
			}
		}
		this.c.doCallBack(c.cb, &ret)
	}
}*/
