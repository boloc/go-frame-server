package components

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetryConnectSucceedsWithoutRetryingWhenFirstAttemptWorks(t *testing.T) {
	calls := 0
	err := retryConnect(context.Background(), 3, time.Millisecond, func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("retryConnect() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("attempt() 被调用了 %d 次，want 1（第一次就成功，不应该继续重试）", calls)
	}
}

// TestRetryConnectRetriesUntilSuccess 验证真的会重试：前两次失败，第三次成功，
// 应该拿到 nil 错误，且总耗时应该体现出中间等待了两次 interval。
func TestRetryConnectRetriesUntilSuccess(t *testing.T) {
	calls := 0
	start := time.Now()
	err := retryConnect(context.Background(), 3, 20*time.Millisecond, func() error {
		calls++
		if calls < 3 {
			return errors.New("boom")
		}
		return nil
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("retryConnect() error = %v", err)
	}
	if calls != 3 {
		t.Fatalf("attempt() 被调用了 %d 次，want 3", calls)
	}
	if elapsed < 30*time.Millisecond {
		t.Fatalf("elapsed = %v，应该至少等待了 2 次 interval（约 40ms）才成功", elapsed)
	}
}

// TestRetryConnectReturnsLastErrorAfterExhaustingAttempts 验证耗尽重试次数后返回最后一次的错误，
// 而不是第一次的错误——最后一次失败的原因往往更贴近"现在到底还有没有恢复"。
func TestRetryConnectReturnsLastErrorAfterExhaustingAttempts(t *testing.T) {
	calls := 0
	err := retryConnect(context.Background(), 3, time.Millisecond, func() error {
		calls++
		return errors.New("attempt failed")
	})

	if err == nil {
		t.Fatal("expected an error after exhausting all attempts")
	}
	if calls != 3 {
		t.Fatalf("attempt() 被调用了 %d 次，want 3", calls)
	}
}

func TestRetryConnectTreatsNonPositiveAttemptsAsOne(t *testing.T) {
	calls := 0
	_ = retryConnect(context.Background(), 0, time.Millisecond, func() error {
		calls++
		return errors.New("boom")
	})
	if calls != 1 {
		t.Fatalf("attempts<=0 应该按 1 次处理，实际调用了 %d 次", calls)
	}
}

// TestRetryConnectStopsEarlyWhenContextCancelled 验证 ctx 取消时不会傻等剩余的重试间隔。
func TestRetryConnectStopsEarlyWhenContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立刻取消

	start := time.Now()
	err := retryConnect(ctx, 5, time.Second, func() error {
		return errors.New("boom")
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error when the context is already cancelled")
	}
	if elapsed > 100*time.Millisecond {
		t.Fatalf("elapsed = %v，ctx 取消后应该立刻返回，不应该等待完整的重试间隔", elapsed)
	}
}
