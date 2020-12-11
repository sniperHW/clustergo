package util

//go test -covermode=count -v -run=.
import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

type fmtLogger struct {
}

func (this *fmtLogger) Debugf(format string, v ...interface{}) { fmt.Printf(format, v...) }
func (this *fmtLogger) Debugln(v ...interface{})               { fmt.Println(v...) }
func (this *fmtLogger) Infof(format string, v ...interface{})  { fmt.Printf(format, v...) }
func (this *fmtLogger) Infoln(v ...interface{})                { fmt.Println(v...) }
func (this *fmtLogger) Warnf(format string, v ...interface{})  { fmt.Printf(format, v...) }
func (this *fmtLogger) Warnln(v ...interface{})                { fmt.Println(v...) }
func (this *fmtLogger) Errorf(format string, v ...interface{}) { fmt.Printf(format, v...) }
func (this *fmtLogger) Errorln(v ...interface{})               { fmt.Println(v...) }
func (this *fmtLogger) Fatalf(format string, v ...interface{}) { fmt.Printf(format, v...) }
func (this *fmtLogger) Fatalln(v ...interface{})               { fmt.Println(v...) }
func (this *fmtLogger) SetLevelByString(level string)          {}

func TestPCall(t *testing.T) {
	f1 := func() {
		var ptr *int
		*ptr = 1
	}

	_, err := ProtectCall(f1)
	assert.NotNil(t, err)
	fmt.Println("----------------err--------------\n", err.Error(), "\n----------------err--------------")

	f2 := func(a int) int {
		stack := CallStack(10)
		fmt.Println("----------------stack of f2--------------\n", stack, "\n----------------stack of f2--------------")
		return a + 2
	}

	ret, err := ProtectCall(f2, 1)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(ret))
	assert.Equal(t, 3, ret[0].(int))

	_, err = ProtectCall(ret)
	assert.Equal(t, ErrArgIsNotFunc, err)

}

func TestRecover(t *testing.T) {
	f1 := func() {
		defer Recover(&fmtLogger{})
		var ptr *int
		*ptr = 1
	}

	f1()

	f2 := func() {
		defer RecoverAndCall(func() { fmt.Println("hello") }, &fmtLogger{})
		var ptr *int
		*ptr = 1
	}

	f2()

}

func TestFormatFileLine(t *testing.T) {
	fmt.Println(FormatFileLine("%d", 1))
}

func TestNotifyer(t *testing.T) {
	notifyer := NewNotifyer()
	notifyer.Notify()
	notifyer.Notify()
	assert.Nil(t, notifyer.Wait())

	time.AfterFunc(time.Second, func() {
		notifyer.Notify()
	})
	assert.Nil(t, notifyer.Wait())

	close(notifyer.notiChan)

	assert.Equal(t, ErrNotifyerClosed, notifyer.Wait())

}

type Ele struct {
	heapIdx int
	value   int
}

func (this *Ele) Less(o HeapElement) bool {
	return this.value < o.(*Ele).value
}

func (this *Ele) GetIndex() int {
	return this.heapIdx
}

func (this *Ele) SetIndex(idx int) {
	this.heapIdx = idx
}

func TestMinHeap(t *testing.T) {
	heap := NewMinHeap(3)

	ele1 := &Ele{value: 10}
	ele2 := &Ele{value: 20}
	ele3 := &Ele{value: 5}

	heap.Insert(ele1)
	heap.Insert(ele2)
	heap.Insert(ele3)

	assert.Equal(t, 3, heap.Size())
	assert.Equal(t, 5, heap.Min().(*Ele).value)

	assert.Equal(t, 5, heap.PopMin().(*Ele).value)
	assert.Equal(t, 10, heap.PopMin().(*Ele).value)
	assert.Equal(t, 20, heap.PopMin().(*Ele).value)

	heap.Insert(ele1)
	heap.Insert(ele2)
	heap.Insert(ele3)

	ele3.value = 100
	heap.Fix(ele3)

	assert.Equal(t, 10, heap.PopMin().(*Ele).value)
	assert.Equal(t, 20, heap.PopMin().(*Ele).value)
	assert.Equal(t, 100, heap.PopMin().(*Ele).value)

	heap.Insert(ele1)
	heap.Insert(ele2)
	heap.Insert(ele3)

	heap.Remove(ele3)
	assert.Equal(t, -1, ele3.GetIndex())
	assert.Equal(t, 2, heap.Size())
	assert.Equal(t, 10, heap.PopMin().(*Ele).value)
	assert.Equal(t, 20, heap.PopMin().(*Ele).value)

	heap.Insert(ele1)
	heap.Insert(ele2)
	heap.Insert(ele3)
	heap.Clear()
	assert.Equal(t, 0, heap.Size())
	assert.Equal(t, -1, ele1.GetIndex())
	assert.Equal(t, -1, ele2.GetIndex())
	assert.Equal(t, -1, ele3.GetIndex())

	heap.Insert(ele1)
	heap.Insert(ele2)
	heap.Insert(ele3)
	ele4 := &Ele{value: 1}
	heap.Insert(ele4)
	assert.Equal(t, 4, heap.Size())

	assert.Equal(t, 1, heap.PopMin().(*Ele).value)
	assert.Equal(t, 10, heap.PopMin().(*Ele).value)
	assert.Equal(t, 20, heap.PopMin().(*Ele).value)
	assert.Equal(t, 100, heap.PopMin().(*Ele).value)

}

func TestBlockQueue(t *testing.T) {
	{
		queue := NewBlockQueue(5)
		assert.Nil(t, queue.Add(1))
		assert.Nil(t, queue.Add(2))
		assert.Nil(t, queue.Add(3))
		assert.Nil(t, queue.Add(4))
		assert.Nil(t, queue.Add(5))
		assert.Equal(t, ErrQueueFull, queue.AddNoWait(6, true))

		queue.Close()

		assert.Equal(t, ErrQueueClosed, queue.Add(6))
	}

	{
		queue := NewBlockQueue(5)
		assert.Nil(t, queue.Add(1))
		assert.Nil(t, queue.Add(2))
		assert.Nil(t, queue.Add(3))
		assert.Nil(t, queue.Add(4))
		assert.Nil(t, queue.Add(5))

		closed, data := queue.Get()
		assert.Equal(t, false, closed)
		assert.Equal(t, 5, len(data))

		assert.Equal(t, 1, data[0].(int))
		assert.Equal(t, 2, data[1].(int))
		assert.Equal(t, 3, data[2].(int))
		assert.Equal(t, 4, data[3].(int))
		assert.Equal(t, 5, data[4].(int))

	}

	{
		queue := NewBlockQueue(5)
		assert.Nil(t, queue.Add(1))
		assert.Nil(t, queue.Add(2))
		assert.Nil(t, queue.Add(3))
		assert.Nil(t, queue.Add(4))
		assert.Nil(t, queue.Add(5))

		assert.Equal(t, false, queue.Closed())
		assert.Equal(t, true, queue.Full())

		queue.Close()

		closed, data := queue.GetNoWait()
		assert.Equal(t, true, closed)
		assert.Equal(t, 5, len(data))

		assert.Equal(t, 1, data[0].(int))
		assert.Equal(t, 2, data[1].(int))
		assert.Equal(t, 3, data[2].(int))
		assert.Equal(t, 4, data[3].(int))
		assert.Equal(t, 5, data[4].(int))

		assert.Equal(t, true, queue.Closed())

	}

	{

		swaped := make([]interface{}, 5)

		queue := NewBlockQueue(5)
		assert.Nil(t, queue.Add(1))
		assert.Nil(t, queue.Add(2))
		assert.Nil(t, queue.Add(3))
		assert.Nil(t, queue.Add(4))
		assert.Nil(t, queue.Add(5))

		closed, data := queue.Swap(swaped)
		assert.Equal(t, false, closed)
		assert.Equal(t, 5, len(data))

		assert.Equal(t, 1, data[0].(int))
		assert.Equal(t, 2, data[1].(int))
		assert.Equal(t, 3, data[2].(int))
		assert.Equal(t, 4, data[3].(int))
		assert.Equal(t, 5, data[4].(int))

		assert.Nil(t, queue.Add(11))
		assert.Equal(t, 11, swaped[0].(int))

	}

	{

		swaped := make([]interface{}, 5)

		queue := NewBlockQueue(5)
		assert.Nil(t, queue.Add(1))
		assert.Nil(t, queue.Add(2))
		assert.Nil(t, queue.Add(3))
		assert.Nil(t, queue.Add(4))
		assert.Nil(t, queue.Add(5))

		ok := make(chan struct{})
		go func() {
			assert.Nil(t, queue.Add(10))
			close(ok)
		}()

		time.Sleep(time.Second)

		queue.SetFullSize(6)

		<-ok

		closed, data := queue.Swap(swaped)
		assert.Equal(t, false, closed)
		assert.Equal(t, 6, len(data))

		assert.Equal(t, 1, data[0].(int))
		assert.Equal(t, 2, data[1].(int))
		assert.Equal(t, 3, data[2].(int))
		assert.Equal(t, 4, data[3].(int))
		assert.Equal(t, 5, data[4].(int))
		assert.Equal(t, 10, data[5].(int))

	}

	{

		swaped := make([]interface{}, 5)

		queue := NewBlockQueue(5)
		assert.Nil(t, queue.Add(1))
		assert.Nil(t, queue.Add(2))
		assert.Nil(t, queue.Add(3))
		assert.Nil(t, queue.Add(4))
		assert.Nil(t, queue.Add(5))

		ok := make(chan struct{})
		go func() {
			assert.Nil(t, queue.Add(10))
			close(ok)
		}()

		time.Sleep(time.Second)

		queue.Clear()

		<-ok

		closed, data := queue.Swap(swaped)
		assert.Equal(t, false, closed)
		assert.Equal(t, 1, len(data))

		assert.Equal(t, 10, data[0].(int))

	}

	{

		queue := NewBlockQueue(5)
		assert.Nil(t, queue.Add(1))
		assert.Nil(t, queue.Add(2))
		assert.Nil(t, queue.Add(3))
		assert.Nil(t, queue.Add(4))
		assert.Nil(t, queue.Add(5))

		ok := make(chan struct{})
		go func() {
			assert.Equal(t, ErrQueueClosed, queue.Add(10))
			close(ok)
		}()

		time.Sleep(time.Second)

		queue.Close()

		<-ok

	}

	{
		queue := NewBlockQueue(5)
		closed, data := queue.GetNoWait()
		assert.Equal(t, false, closed)
		assert.Equal(t, 0, len(data))
	}

	{
		ok := make(chan struct{})
		queue := NewBlockQueue(5)
		go func() {
			closed, data := queue.Get()
			assert.Equal(t, false, closed)
			assert.Equal(t, 1, len(data))
			close(ok)
		}()

		time.Sleep(time.Second)

		assert.Nil(t, queue.Add(1))

		<-ok

	}

}
