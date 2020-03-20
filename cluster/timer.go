package cluster

import (
	"github.com/sniperHW/kendynet/timer"
	"time"
)

func RegisterTimerOnce(expired time.Time, callback func(*timer.Timer, interface{}), ctx interface{}) *timer.Timer {
	return timer.Once(expired.Sub(time.Now()), GetEventQueue(), callback, ctx)
}

func RegisterTimer(timeout time.Duration, callback func(*timer.Timer, interface{}), ctx interface{}) *timer.Timer {
	return timer.Repeat(timeout, GetEventQueue(), callback, ctx)
}
