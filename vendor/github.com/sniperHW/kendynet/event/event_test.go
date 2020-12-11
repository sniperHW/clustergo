package event

//go test -covermode=count -v -run=.
import (
	"fmt"
	"testing"
)

func testQueueMode() {

	fmt.Println("-------------------testQueueMode-----------------")

	{
		fmt.Println("1")
		handler := NewEventHandler()

		h1 := handler.Register("queue", func() {
			panic("handler1")
		})

		handler.Remove(h1)

		handler.Emit("queue")
	}

	{
		fmt.Println("2")
		handler := NewEventHandler()

		h1 := handler.Register("queue", func() {
			fmt.Println("handler1")
			//在第一个handler里调用了clear,后面的handler2,和handler3不会执行
			handler.Clear("queue")
			//注册新handler，将在下一次emit执行
			handler.Register("queue", func() {
				fmt.Println("handler11")
			})
		})

		handler.Register("queue", func() {
			panic("handler2")
		})

		handler.Register("queue", func() {
			panic("handler3")
		})

		/*
		 * 所有注册的处理器将按注册顺序依次执行
		 */

		handler.Emit("queue")

		fmt.Println("again")

		handler.Emit("queue")

		handler.Remove(h1)

	}

	{
		fmt.Println("3")
		handler := NewEventHandler()

		handler.Register("queue", func() {
			fmt.Println("handler1")
		})

		handler.Register("queue", func() {
			fmt.Println("handler2")
			handler.Register("queue", func() {
				fmt.Println("handler21")
			})
		})

		handler.Register("queue", func() {
			fmt.Println("handler3")
		})

		/*
		 * 所有注册的处理器将按注册顺序依次执行
		 */

		handler.Emit("queue")

		fmt.Println("again")

		handler.Emit("queue")

		fmt.Println("again")

		handler.Clear("queue")

		handler.Register("queue", func() {
			fmt.Println("handler11")
		})

		handler.Emit("queue")
	}

}

func testQueueOnceMode() {

	fmt.Println("-------------------testQueueOnceMode-----------------")

	handler := NewEventHandler()

	handler.Register("queue", func(h Handle, msg ...interface{}) {
		fmt.Println("handler1", msg[0])
		handler.Remove(h)
	})

	handler.RegisterOnce("queue", func(h Handle) {
		fmt.Println("handler2")
	})

	h3 := handler.Register("queue", func() {
		fmt.Println("handler3")
	})

	handler.Register("queue", func(msg ...interface{}) {
		fmt.Println("handler4", msg[0])
	})

	/*
	 * 所有注册的处理器将按注册顺序依次执行
	 */

	handler.Emit("queue", "hello")

	fmt.Println("again")

	//再次触发事件，因为handler2被注册为只触发一次，此时handler2已经被删除，所以不会再次被调用

	handler.Emit("queue", "world")

	handler.Remove(h3)

	fmt.Println("again")

	handler.Emit("queue", "world")

}

func testUseEventQueue() {
	queue := NewEventQueue()
	go queue.Run()

	handler := NewEventHandler(queue)

	handler.Register("queue", func() {
		fmt.Println("handler1")

		queue.Post(func() {
			fmt.Println("queue fun1")
		})

		queue.PostFullReturn(func([]interface{}) {
			fmt.Println("queue fun2")
		})

		queue.PostNoWait(func(...interface{}) {
			fmt.Println("queue fun3")
			queue.Close()
		})

	})

	handler.Emit("queue")

}

func TestEvent(t *testing.T) {
	testQueueMode()

	testQueueOnceMode()

	testUseEventQueue()
}
