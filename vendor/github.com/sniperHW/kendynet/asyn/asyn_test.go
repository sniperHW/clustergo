package asyn

//go test -covermode=count -v -run=.
import (
	"context"
	"fmt"
	"github.com/sniperHW/kendynet/event"
	"github.com/stretchr/testify/assert"
	"sync"
	"testing"
	"time"
)

func mySleep1() int {
	fmt.Println("mySleep1 sleep")
	time.Sleep(time.Second)
	fmt.Println("mySleep1 wake")
	return 1
}

func mySleep2(s int) int {
	fmt.Println("mySleep2 sleep")
	time.Sleep(time.Second * time.Duration(s))
	fmt.Println("mySleep2 wake")
	return 2
}

type st struct {
	data int
}

func (this *st) fun() {
	time.Sleep(time.Second * 3)
	fmt.Println("fun", this.data)
}

func TestAsyn(t *testing.T) {
	{
		//all
		begUnix := time.Now().Unix()
		ret, err := Paralell(
			func(_ context.Context) interface{} {
				time.Sleep(time.Second * 1)
				return 1
			},
			func(_ context.Context) interface{} {
				time.Sleep(time.Second * 2)
				return 2
			},
			func(_ context.Context) interface{} {
				time.Sleep(time.Second * 3)
				return 3
			},
		).Wait()

		assert.Nil(t, err)
		assert.Equal(t, time.Now().Unix()-begUnix, int64(3))
		assert.Equal(t, len(ret), 3)
		assert.Equal(t, 1, ret[0].(int))
		assert.Equal(t, 2, ret[1].(int))
		assert.Equal(t, 3, ret[2].(int))
	}

	{
		//any
		begUnix := time.Now().Unix()
		ret, err := Paralell(
			func(_ context.Context) interface{} {
				time.Sleep(time.Second * 1)
				return 1
			},
			func(_ context.Context) interface{} {
				time.Sleep(time.Second * 2)
				return 2
			},
			func(_ context.Context) interface{} {
				time.Sleep(time.Second * 3)
				return 3
			},
		).WaitAny()

		assert.Nil(t, err)
		assert.Equal(t, time.Now().Unix()-begUnix, int64(1))
		assert.Equal(t, 1, ret.(int))
	}

	{
		wg := &sync.WaitGroup{}
		begUnix := time.Now().Unix()
		_, err := Paralell(
			func(ctx context.Context) interface{} {
				defer wg.Done()
				wg.Add(1)
				for i := 1; i < 3; i++ {
					select {
					case <-ctx.Done():
						fmt.Printf("stop 1\n")
						return nil
					default:
						time.Sleep(time.Second * 2)
					}
				}
				return 1
			},
			func(ctx context.Context) interface{} {
				defer wg.Done()
				wg.Add(1)
				for i := 1; i < 3; i++ {
					select {
					case <-ctx.Done():
						fmt.Printf("stop 2\n")
						return nil
					default:
						time.Sleep(time.Second * 2)
					}
				}
				return 2
			},
			func(ctx context.Context) interface{} {
				defer wg.Done()
				wg.Add(1)
				for i := 1; i < 3; i++ {
					select {
					case <-ctx.Done():
						fmt.Printf("stop 3\n")
						return nil
					default:
						time.Sleep(time.Second * 2)
					}
				}
				return 3
			},
		).Wait(time.Second * 1)

		assert.Equal(t, time.Now().Unix()-begUnix, int64(1))
		assert.Equal(t, err, ErrTimeout)

		wg.Wait()

	}

	{
		future := Paralell(
			func(_ context.Context) interface{} {
				time.Sleep(time.Second * 1)
				return 1
			},
			func(_ context.Context) interface{} {
				time.Sleep(time.Second * 2)
				return 2
			},
			func(_ context.Context) interface{} {
				time.Sleep(time.Second * 3)
				return 3
			},
		)

		time.Sleep(time.Second * 5)

		//5秒之后等待执行结果，应该立即返回，应为在这一点所有闭包都已执行完毕
		begUnix := time.Now().Unix()
		ret, err := future.Wait()

		assert.Nil(t, err)
		assert.Equal(t, time.Now().Unix()-begUnix, int64(0))
		assert.Equal(t, len(ret), 3)
		assert.Equal(t, 1, ret[0].(int))
		assert.Equal(t, 2, ret[1].(int))
		assert.Equal(t, 3, ret[2].(int))

	}

	{
		SetRoutinePool(NewRoutinePool(1024))

		queue := event.NewEventQueue()
		s := st{data: 100}

		wrap1 := AsynWrap(queue, mySleep1)
		wrap2 := AsynWrap(queue, mySleep2)
		wrap3 := AsynWrap(queue, s.fun)

		wrap1(func(ret []interface{}) {
			fmt.Println(ret[0].(int))
		})

		wrap2(func(ret []interface{}) {
			fmt.Println(ret[0].(int))
		}, 2)

		wrap3(func(ret []interface{}) {
			fmt.Println("st.fun callback")
			queue.Close()
		})

		queue.Run()
	}
}
