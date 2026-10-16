package onec

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
)

// maxWorkers — сколько ядер занимать разбором большой конфигурации.
var maxWorkers = min(runtime.NumCPU(), 8)

// forEach вызывает fn(i) для i в [0, n) на нескольких горутинах. Первая ошибка
// или отмена ctx останавливает раздачу работы; возвращается первая ошибка.
func forEach(ctx context.Context, n int, fn func(i int) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	workers := min(maxWorkers, n)
	if workers <= 1 {
		for i := 0; i < n; i++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := fn(i); err != nil {
				return err
			}
		}
		return nil
	}
	var (
		next     atomic.Int64
		stop     atomic.Bool
		once     sync.Once
		firstErr error
		wg       sync.WaitGroup
	)
	fail := func(err error) {
		once.Do(func() { firstErr = err })
		stop.Store(true)
	}
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stop.Load() {
				i := int(next.Add(1) - 1)
				if i >= n {
					return
				}
				if i%256 == 0 {
					if err := ctx.Err(); err != nil {
						fail(err)
						return
					}
				}
				if err := fn(i); err != nil {
					fail(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	return firstErr
}
