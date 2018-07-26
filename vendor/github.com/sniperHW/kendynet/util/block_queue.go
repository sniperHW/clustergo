package util

import (
	"sync"
	"fmt"
)

var (
	ErrQueueClosed = fmt.Errorf("queue closed")
)

type BlockQueue struct {
	list      [] interface{}
	listGuard sync.Mutex
	listCond  *sync.Cond
	closed     bool
	waited     int
}

func (self *BlockQueue) Add(item interface{}) error {
	self.listGuard.Lock()
	if self.closed {
		self.listGuard.Unlock()
		return ErrQueueClosed
	}
	n := len(self.list)
	self.list = append(self.list, item)
	needSignal := self.waited > 0 && n == 0/*BlockQueue目前主要用于单消费者队列，这里n == 0的处理是为了这种情况的优化,减少Signal的调用次数*/
	self.listGuard.Unlock()
	if needSignal {
		self.listCond.Signal()
	}
	return nil
}

func (self *BlockQueue) Closed() bool {
	var closed bool
	self.listGuard.Lock()
	closed = self.closed
	self.listGuard.Unlock()
	return closed
}

func (self *BlockQueue) Get() (closed bool,datas []interface{}) {
	self.listGuard.Lock()
	for !self.closed && len(self.list) == 0 {
		//Cond.Wait不能设置超时，蛋疼
		self.waited++
		self.listCond.Wait()
		self.waited--
	}
	if len(self.list) > 0 {
		datas  = self.list
		self.list = make([]interface{},0)
	}

	closed = self.closed
	self.listGuard.Unlock()
	return
}

func (self *BlockQueue) Swap(swaped []interface{}) (closed bool,datas []interface{}) {
	swaped = swaped[0:0]
	self.listGuard.Lock()
	for !self.closed && len(self.list) == 0 {
		self.waited++
		//Cond.Wait不能设置超时，蛋疼
		self.listCond.Wait()
		self.waited--
	}
	datas  = self.list
	closed = self.closed
	self.list = swaped
	self.listGuard.Unlock()
	return
}

func (self *BlockQueue) Close() {
	self.listGuard.Lock()

	if self.closed {
		self.listGuard.Unlock()
		return
	}

	self.closed = true
	self.listGuard.Unlock()
	self.listCond.Signal()
}

func (self *BlockQueue) Len() (length int) {
	self.listGuard.Lock()
	length = len(self.list)
	self.listGuard.Unlock()
	return
}

func (self *BlockQueue) Clear() {
	self.listGuard.Lock()
	self.list = self.list[0:0]
	self.listGuard.Unlock()
	return
}

func NewBlockQueue() *BlockQueue {
	self := &BlockQueue{}
	self.closed = false
	self.listCond = sync.NewCond(&self.listGuard)

	return self
}