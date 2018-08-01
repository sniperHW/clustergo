package event

import(
	"sync"
	"fmt"
)

const (
	tt_noargs  = 1   //无参回调
	tt_varargs = 2   //不定参数回调
)

const (
	Mode_once        = 1 << 1   //一次性，handler触发之后被移除
	Mode_queue   	 = 1 << 2   //队列模式，事件触发时按数组顺序依次执行handler
	Mode_exclusive   = 1 << 3   //排它模式，只有队列尾部的handler被触发 
)

type HandlerID int64

type handler struct {
	id            HandlerID
	tt            int
	args          []interface{}
	callback      interface{}
	mode          int
	prev         *handler
	next         *handler
}

type handlerSlot struct {
	head     	handler
	tail     	handler
	nextID   	int64
	mtx      	sync.Mutex
	emitting  	bool
}

func (this *handlerSlot) register(mode int,callback interface{}) (HandlerID,error) {
	
	once  	  := false
	exclusive := false
	queue     := false

	if mode & Mode_once > 0 {
		once = true
	}

	if mode & Mode_exclusive > 0 {
		exclusive = true
	}

	if mode & Mode_queue > 0 {
		queue = true
	}

	if !once && !exclusive && !queue {
		return HandlerID(0),fmt.Errorf("invaild mode:%d",mode)
	}

	if exclusive && queue {
		return HandlerID(0),fmt.Errorf("invaild mode:%d",mode)		
	}

	var tt int

	switch callback.(type) {
	case func():
		tt = tt_noargs
		break
	case func([]interface{}):
		tt = tt_varargs
		break
	default:
		return HandlerID(0),fmt.Errorf("invaild callback")	
	}

	defer this.mtx.Unlock()
	this.mtx.Lock()

	last := this.tail.prev

	if last.mode & Mode_exclusive > 0 && !exclusive {
		//尾部的handler处于mutex模式，禁止向尾部添加非mutex模式的handler
		return HandlerID(0),fmt.Errorf("tail in exclusive mode")
	}
	this.nextID++
	h := &handler{
		id 		 : HandlerID(this.nextID),
		tt 		 : tt,
		callback : callback,
		mode     : mode,	
	}

	h.next = &this.tail
	h.prev = this.tail.prev

	this.tail.prev.next = h
	this.tail.prev = h

	return h.id,nil

}

func (this *handlerSlot) emit(args ...interface{}) {

	this.mtx.Lock()
	if this.emitting {
		this.mtx.Unlock()
		//在handler中再次emit事件(死循环)
		panic("emit recursive")
	}
	last := this.tail.prev
	if last == &this.head {
		//empty
		this.mtx.Unlock()
		return
	}
	this.emitting = true
	handlers := make([]*handler,0)
	if last != &this.head {
		if last.mode & Mode_exclusive > 0 {
			handlers = append(handlers,last)
		} else {
			cur := this.head.next
			for cur != &this.tail {
				handlers = append(handlers,cur)
				cur = cur.next
			}
		}
	}
	this.mtx.Unlock()	

	//必须在不持有锁的状态下执行handler
	for _,v := range(handlers) {
		pcall(v.tt,v.callback,v.args)
		if v.mode & Mode_once > 0 {
			//一次性，执行完之后要删除
			this.remove(v.id)
		}
	}

	this.mtx.Lock()
	this.emitting = false
	this.mtx.Unlock()

}

func (this *handlerSlot) clear() {
	defer this.mtx.Unlock()
	this.mtx.Lock()
	this.head.next = &this.tail
	this.tail.prev = &this.head 
}

func (this *handlerSlot) remove(id HandlerID) {


	defer this.mtx.Unlock()
	this.mtx.Lock()

	cur := this.head.next

	for cur != &this.tail {
		if cur.id == id {
			break
		} else {
			cur = cur.next
		}
	}

	if cur != &this.tail {
		cur.prev.next = cur.next
		cur.next.prev = cur.prev
	}
}

type EventHandler struct {
	mtx   sync.Mutex
	slots map[interface{}]*handlerSlot
}


func (this *EventHandler) Register(mode int,event interface{},callback interface{}) (HandlerID,error) {

	this.mtx.Lock()
	slot,ok := this.slots[event]
	if !ok {
		slot = &handlerSlot{
			nextID : 0,
		}

		slot.head.next = &slot.tail
		slot.tail.prev = &slot.head

		this.slots[event] = slot 

	}
	this.mtx.Unlock()
	return slot.register(mode,callback)

}

func (this *EventHandler) Remove(event interface{},id HandlerID) {
	this.mtx.Lock()
	slot,ok := this.slots[event]
	this.mtx.Unlock()
	if ok {
		slot.remove(id)
	}	
}

func (this *EventHandler) Clear(event interface{}) {
	this.mtx.Lock()
	slot,ok := this.slots[event]
	this.mtx.Unlock()
	if ok {
		slot.clear()
	}
}

//触发事件
func (this *EventHandler) Emit(event interface{},args ...interface{}) {
	this.mtx.Lock()
	slot,ok := this.slots[event]
	this.mtx.Unlock()
	if ok {
		slot.emit(args...)
	}
}


func NewEventHandler() *EventHandler {
	return &EventHandler{
		slots : map[interface{}]*handlerSlot{},
	}
}