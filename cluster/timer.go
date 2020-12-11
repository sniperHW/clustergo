package cluster

import (
	"github.com/sniperHW/kendynet/timer"
	"time"
)

//timer回调只有在Cluster的运行状态下才会执行。

func (this *Cluster) RegisterTimerOnce(expired time.Time, callback func(*timer.Timer, interface{}), ctx interface{}) *timer.Timer {
	return timer.Once(expired.Sub(time.Now()), this.queue, callback, ctx)
}

func (this *Cluster) RegisterTimer(timeout time.Duration, callback func(*timer.Timer, interface{}), ctx interface{}) *timer.Timer {
	return timer.Repeat(timeout, this.queue, callback, ctx)
}
