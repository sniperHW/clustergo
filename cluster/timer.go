package cluster

import (
	"runtime"
	"sync"
	"time"

	"github.com/sniperHW/kendynet/util"
)

const (
	timer_register   = 1 //正在注册
	timer_unregister = 2 //被反注册
	timer_docallback = 3 //正在执行回调
	timer_watting    = 4 //尚未到时
)

type Timer struct {
	heapIndex uint32
	tt        int
	args      []interface{}
	callback  interface{}
	expired   time.Time
	status    int
	mtx       sync.Mutex
}

func (this *Timer) GetIndex() uint32 {
	return this.heapIndex
}

func (this *Timer) SetIndex(idx uint32) {
	this.heapIndex = idx
}

func (this *Timer) Less(o util.HeapElement) bool {
	return this.expired.Before(o.(*Timer).expired)
}

func (this *Timer) do() {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 65535)
			l := runtime.Stack(buf, false)
			Errorf("%v: %s\n", r, buf[:l])
		}
	}()

	if this.tt == 0 {
		this.callback.(func())()
	} else {
		this.callback.(func([]interface{}))(this.args)
	}
}

var minheap *util.MinHeap = util.NewMinHeap(4096)

func RegisterTimer(expired time.Time, callback interface{}, args ...interface{}) *Timer {

	t := &Timer{}

	switch callback.(type) {
	case func():
		t.tt = 0
		t.callback = callback
		break
	case func([]interface{}):
		t.tt = 1
		t.callback = callback
		t.args = args
		break
	default:
		panic("invaild callback type")
	}
	t.expired = expired
	t.status = timer_register

	//投递到cluster主消息循环中执行
	queue.PostNoWait(func() {
		defer t.mtx.Unlock()
		t.mtx.Lock()
		if t.status == timer_unregister {
			//已经被取消
			return
		}
		t.status = timer_watting
		minheap.Insert(t)
	})
	return t
}

func UnregisterTimer(t *Timer) bool {
	defer t.mtx.Unlock()
	t.mtx.Lock()
	if t.status == timer_register || t.status == timer_watting {
		t.status = timer_unregister
		queue.PostNoWait(func() {
			minheap.Remove(t)
		})
		return true
	}
	return false
}

func TickTimer() {
	now := time.Now()
	for {
		r := minheap.Min()
		if r != nil && now.After(r.(*Timer).expired) {
			minheap.PopMin()
			t := r.(*Timer)
			t.mtx.Lock()
			if t.status == timer_watting {
				t.status = timer_docallback
				t.mtx.Unlock()
				t.do()
				t.mtx.Lock()
			}
			t.status = timer_unregister
			t.mtx.Unlock()
		} else {
			break
		}
	}
}
