package main

/*
*  展示如何将一个同步调用转换成基于callback的接口
*/

import(
	"github.com/sniperHW/kendynet/asyn"
	"github.com/sniperHW/kendynet/event"
	"time"
	"fmt"
)


func blockCall(arg string) string {
	fmt.Println("blockCall")
	//模拟耗时操作
	time.Sleep(time.Second*5)
	return arg + " " + "world"
}


func main() {

	counter := 0

	/* 设置go程池,将同步任务交给go程池执行
	*  如果没有设置，每个任务都会创建一个单独的go程去执行
	*/
	asyn.SetRoutinePool(asyn.NewRoutinePool(1024))

	queue := event.NewEventQueue()

	/*
	*  创建一个包装函数，将同步阻塞函数转换成基于异步回调的函数
	*  queue：用于执行回调的任务队列
	*/
	wrap := asyn.AsynWrap(queue,blockCall)

	/*
	*  执行调用，原函数在一个独立的go程中执行，不会阻塞当前go程
	*  当原函数返回后，将回调投递到事件队列执行
	*/
	wrap(func(ret []interface{}) {
		fmt.Println(ret[0].(string))
		counter++
		if counter >= 5 {
			queue.Close()
		}
	},"hello1")

	wrap(func(ret []interface{}) {
		fmt.Println(ret[0].(string))
		counter++
		if counter >= 5 {
			queue.Close()
		}
	},"hello2")

	wrap(func(ret []interface{}) {
		fmt.Println(ret[0].(string))
		counter++
		if counter >= 5 {
			queue.Close()
		}
	},"hello3")

	wrap(func(ret []interface{}) {
		fmt.Println(ret[0].(string))
		counter++
		if counter >= 5 {
			queue.Close()
		}
	},"hello4")

	wrap(func(ret []interface{}) {
		fmt.Println(ret[0].(string))
		counter++
		if counter >= 5 {
			queue.Close()
		}
	},"hello5")

	fmt.Println("after call")

	queue.Run()
}

