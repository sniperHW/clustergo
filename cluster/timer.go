package cluster

import (
	"github.com/sniperHW/kendynet/timer"
	"time"
)

func RegisterTimerOnce(expired time.Time, callback func(*timer.Timer)) *timer.Timer {
	return timer.Once(expired.Sub(time.Now()), GetEventQueue(), callback)
}

func RegisterTimer(timeout time.Duration, callback func(*timer.Timer)) *timer.Timer {
	return timer.Repeat(timeout, GetEventQueue(), callback)
}
