package asyn


/*
*  将同步接函数调用转换成基于回调的接口
*/ 

import(
	"github.com/sniperHW/kendynet"
	"reflect"
)

var routinePool_ * routinePool

type caller struct {
	queue     *kendynet.EventQueue
	oriFunc	   reflect.Value
}

func (this *caller) Call(callback func([]interface{}),args ...interface{}) {

	f := func () {
		in := []reflect.Value{}
		for _,v := range(args) {
			in = append(in,reflect.ValueOf(v))
		}
		out := this.oriFunc.Call(in) 
		ret := []interface{}{}
		for _,v := range(out) {
			ret = append(ret,v.Interface())
		}
		this.queue.Post(callback,ret...)
	}

	if nil == routinePool_ {
		go f()
	} else {
		//设置了go程池，交给go程池执行
		routinePool_.AddTask(f)
	}
}

func AsynWrap(queue *kendynet.EventQueue,oriFunc interface{}) *caller {

	if nil == queue {
		return nil
	}

	v := reflect.ValueOf(oriFunc)

	if v.Kind() != reflect.Func {
		return nil
	}

	return &caller{
		queue : queue,
		oriFunc : v,
	}
}

func SetRoutinePool(pool *routinePool) {
	routinePool_ = pool
}

