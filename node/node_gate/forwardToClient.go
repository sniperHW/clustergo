package node_gate

import (
	"sync/atomic"
)

//将来自内部服务器的消息直接透传到客户端

func (this *gateUser) RelaySCMessage(bytes [][]byte) error {
	Debugln("RelaySCMessage", this.userID, this.status, this.conn)
	this.mtx.Lock()
	defer this.mtx.Unlock()
	for _, msg := range bytes {
		err := this.downQueue.append(msg)
		if nil != err {
			return err
		}
	}
	if this.status == status_ok && nil != this.conn {
		Debugln("RelaySCMessage flush", this.userID)
		this.FlushSCMessage()
	}
	return nil
}

func (this *gateUser) FlushSCMessage() {
	if nil == this.conn {
		Debugln("RelaySCMessage flush return 1", this.userID)
		return
	}

	if status_ok != this.status {
		Debugln("RelaySCMessage flush return 2", this.userID)
		return
	}

	for i, v := range this.downQueue.queue {
		err := this.conn.Send(v)
		if nil != err {
			Debugln(err)
			q := make([][]byte, 0, len(this.downQueue.queue)-i)
			copy(q, this.downQueue.queue[i:])
			this.downQueue.queue = q
			return
		} else {
			this.downQueue.byteSize -= int64(len(v))
			atomic.AddInt64(&totalSendQueueBytes, -int64(len(v)))
		}
	}
	this.downQueue.queue = [][]byte{}
}
