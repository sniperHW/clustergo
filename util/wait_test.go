package util

import (
	"github.com/sniperHW/kendynet/event"
	"testing"
)

func TestWait(t *testing.T) {

	WaitCondition(nil, func() bool {
		return true
	})

	q := event.NewEventQueue()
	go q.Run()
	WaitCondition(q, func() bool {
		return true
	})

}
