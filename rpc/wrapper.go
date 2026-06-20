package rpc

import (
	"context"
)

/*
 * 方法包装策略（Handler Wrappers）
 *
 * 默认情况下，RPC 方法在底层信道的接收协程上同步执行（见 Server.OnMessage）。
 * 单个慢方法会阻塞该连接的所有后续请求。
 *
 * 这里提供的策略对象独立构造，可被多个方法（甚至不同 Arg 类型）共享，
 * 从而实现跨方法/跨类型的统一限流与并发控制。框架不写死任何策略，
 * 使用者在 Register 时按需调用策略对象的 Wrap 方法包装业务方法。
 *
 * 用法：
 *
 *	pool := rpc.NewAsyncPool(4, 64) // 4 workers，队列容量 64
 *	rpc.Register(s, "echo",  pool.Wrap(func(c, r, a *EchoArg) { ... }))
 *	rpc.Register(s, "heavy", pool.Wrap(func(c, r, a *HeavyArg) { ... }))
 *
 * 两个方法共享同一个 worker 池，任一方法占满队列，另一个也会收到 ErrBusy。
 */

// AsyncPool 是一个共享的 worker 池，可被多个 RPC 方法（包括不同 Arg 类型）
// 共同复用。请求到达时尝试投递到任务队列：
//   - 投递成功：原方法在某个 worker 协程上异步执行，接收协程立即返回，不被业务阻塞。
//   - 队列已满：立即回复 ErrBusy，不阻塞接收协程。
//
// workers 为并发执行协程数，queue 为待执行任务的缓冲容量。
type AsyncPool struct {
	tasks chan func()
}

// NewAsyncPool 创建一个共享 worker 池。workers <= 0 或 queue <= 0 时返回 nil，
// 此时 Wrap 返回原方法（不做异步，回退为同步执行）。
func NewAsyncPool(workers, queue int) *AsyncPool {
	if workers <= 0 || queue <= 0 {
		return nil
	}
	p := &AsyncPool{tasks: make(chan func(), queue)}
	for i := 0; i < workers; i++ {
		go func() {
			for task := range p.tasks {
				task()
			}
		}()
	}
	return p
}

// Wrap 把业务方法 fn 派发到共享 worker 池异步执行。多个方法（含不同 Arg 类型）
// 可复用同一个 AsyncPool，实现统一的并发与队列限流。
//
// 注意：Go 方法不允许带类型参数，因此 Wrap 是顶层泛型函数（pool 作为参数传入），
// 而非 AsyncPool 的方法。
func Wrap[Arg any](p *AsyncPool, fn func(context.Context, *Replyer, *Arg)) func(context.Context, *Replyer, *Arg) {
	if p == nil {
		return fn
	}
	return func(c context.Context, r *Replyer, a *Arg) {
		select {
		case p.tasks <- func() { fn(c, r, a) }:
		default:
			r.Error(NewError(ErrBusy, "busy"))
		}
	}
}
