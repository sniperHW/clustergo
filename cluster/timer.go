package cluster

import (
	"github.com/sniperHW/kendynet/timer"
	"github.com/sniperHW/sanguo/cluster/priority"
	"time"
)

//timer回调只有在Cluster的运行状态下才会执行。

func (this *Cluster) RegisterTimerOnce(timeout time.Duration, callback func(*timer.Timer, interface{}), ctx interface{}) *timer.Timer {
	return timer.Once(timeout, func(t *timer.Timer, ctx interface{}) {
		this.queue.PostNoWait(priority.MID, callback, t, ctx)
	}, ctx)
}

func (this *Cluster) RegisterTimer(timeout time.Duration, callback func(*timer.Timer, interface{}), ctx interface{}) *timer.Timer {
	return timer.Repeat(timeout, func(t *timer.Timer, ctx interface{}) {
		this.queue.PostNoWait(priority.MID, callback, t, ctx)
	}, ctx)
}
