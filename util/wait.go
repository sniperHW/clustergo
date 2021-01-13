package util

import (
	"github.com/sniperHW/kendynet/event"
	"sync"
	"sync/atomic"
	"time"
)

func WaitCondition(eventq *event.EventQueue, fn func() bool) {
	wg := sync.WaitGroup{}
	wg.Add(1)

	donefire := int32(0)

	if nil == eventq {
		go func() {
			for {
				time.Sleep(time.Millisecond * 100)
				if fn() {
					if atomic.LoadInt32(&donefire) == 0 {
						atomic.StoreInt32(&donefire, 1)
						wg.Done()
					}
					break
				}
			}
		}()
	} else {
		go func() {
			stoped := int32(0)
			for atomic.LoadInt32(&stoped) == 0 {
				time.Sleep(time.Millisecond * 100)
				eventq.PostNoWait(func() {
					if fn() {
						if atomic.LoadInt32(&donefire) == 0 {
							atomic.StoreInt32(&donefire, 1)
							wg.Done()
						}
						atomic.StoreInt32(&stoped, 1)
					}
				})
			}
		}()
	}

	wg.Wait()
}
