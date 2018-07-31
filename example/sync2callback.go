package main

/*
*  展示如何将一个同步调用转换成基于callback的接口
*/

import(
	"github.com/sniperHW/kendynet/util/asyn"
	"github.com/sniperHW/kendynet"
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

	/* 设置go程池,将同步任务交给go程池执行
	*  如果没有设置，每个任务都会创建一个单独的go程去执行
	*/
	asyn.SetRoutinePool(asyn.NewRoutinePool(1024))

	queue := kendynet.NewEventQueue()

	//创建一个blockCall的包装器，并绑定处理队列
	caller := asyn.AsynWrap(queue,blockCall)

	/*
	*  执行调用，原函数在一个独立的go程中执行，不会阻塞当前go程
	*  当原函数返回后，将回调投递到事件队列执行
	*/
	caller.Call(func(ret []interface{}) {
		fmt.Println(ret[0].(string))
		queue.Close()
	},"hello")


	queue.Run()
}

