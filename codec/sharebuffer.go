package codec

import (
	"sync"
)

//实现goaio.ShareBuffer接口

type BufferPool struct {
	pool       sync.Pool
	bufferSize uint64
}

func NewBufferPool(bufferSize uint64) *BufferPool {
	if 0 == bufferSize {
		bufferSize = 64 * 1024
	}
	return &BufferPool{
		bufferSize: bufferSize,
		pool: sync.Pool{
			New: func() interface{} {
				return make([]byte, bufferSize)
			},
		},
	}
}

func (p *BufferPool) Acquire() []byte {
	return p.pool.Get().([]byte)
}

func (p *BufferPool) Release(buff []byte) {
	if uint64(cap(buff)) == p.bufferSize {
		p.pool.Put(buff[:cap(buff)])
	}
}
