package components

import (
	"context"
	"time"
)

// retryConnect 按 attempts/interval 重试 attempt，直到成功或耗尽次数；ctx 取消时提前退出。
// attempts<=0 时按 1 次处理。
func retryConnect(ctx context.Context, attempts int, interval time.Duration, attempt func() error) error {
	if attempts <= 0 {
		attempts = 1
	}

	var lastErr error
	for i := 0; i < attempts; i++ {
		if err := attempt(); err == nil {
			return nil
		} else {
			lastErr = err
		}

		if i == attempts-1 {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
	return lastErr
}
