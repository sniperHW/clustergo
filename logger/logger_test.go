package logger

import (
	"fmt"
	"sync"
	"testing"
)

// recLogger 记录收到的日志调用，用于断言转发是否生效。
type recLogger struct {
	mu    sync.Mutex
	calls []string
}

func (r *recLogger) add(format string, args ...interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, fmt.Sprintf(format, args...))
}

func (r *recLogger) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

func (r *recLogger) Debugf(f string, a ...interface{}) { r.add(f, a...) }
func (r *recLogger) Infof(f string, a ...interface{})  { r.add(f, a...) }
func (r *recLogger) Warnf(f string, a ...interface{})  { r.add(f, a...) }
func (r *recLogger) Errorf(f string, a ...interface{}) { r.add(f, a...) }
func (r *recLogger) Panicf(f string, a ...interface{}) { r.add(f, a...) }
func (r *recLogger) Fatalf(f string, a ...interface{}) { r.add(f, a...) }
func (r *recLogger) Debug(a ...interface{})            { r.add("%v", fmt.Sprint(a...)) }
func (r *recLogger) Info(a ...interface{})             { r.add("%v", fmt.Sprint(a...)) }
func (r *recLogger) Warn(a ...interface{})             { r.add("%v", fmt.Sprint(a...)) }
func (r *recLogger) Error(a ...interface{})            { r.add("%v", fmt.Sprint(a...)) }
func (r *recLogger) Panic(a ...interface{})            { r.add("%v", fmt.Sprint(a...)) }
func (r *recLogger) Fatal(a ...interface{})            { r.add("%v", fmt.Sprint(a...)) }

func TestInitLoggerForwards(t *testing.T) {
	rec := &recLogger{}
	InitLogger(rec)
	t.Cleanup(func() { InitLogger(nil) })

	if Get() != rec {
		t.Fatal("Get did not return the injected logger")
	}

	Errorf("code:%d", 7)
	Info("hello")
	if got := rec.snapshot(); len(got) != 2 || got[0] != "code:7" || got[1] != "hello" {
		t.Fatalf("unexpected forwarded calls: %v", got)
	}
}

func TestNilLoggerFallsBackToNop(t *testing.T) {
	InitLogger(nil)
	t.Cleanup(func() { InitLogger(nil) })

	// 未初始化时普通日志被丢弃，不应 panic。
	Debugf("dropped %d", 1)
	Debug("dropped")
	Errorf("dropped")
}

func TestNilLoggerStillPanicsOnPanicAndFatal(t *testing.T) {
	InitLogger(nil)
	t.Cleanup(func() { InitLogger(nil) })

	for _, fn := range []func(){
		func() { Panicf("boom %d", 1) },
		func() { Fatalf("boom %d", 1) },
		func() { Panic("boom") },
		func() { Fatal("boom") },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatal("expected panic from Panic/Fatal fallback")
				}
			}()
			fn()
		}()
	}
}

// TestConcurrentInitAndLog 在 -race 下验证运行期替换日志器与并发写日志无数据竞争。
func TestConcurrentInitAndLog(t *testing.T) {
	t.Cleanup(func() { InitLogger(nil) })

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			InitLogger(&recLogger{})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			Infof("n:%d", i)
			_ = Get()
		}
	}()
	wg.Wait()
}
