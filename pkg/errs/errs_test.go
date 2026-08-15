package errs_test

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"

	. "github.com/boloc/go-frame-server/pkg/errs"
)

func TestFromExtractsWrappedError(t *testing.T) {
	original := New(CodeNotFound, "not found")
	wrapped := errors.Join(errors.New("context"), original)

	got := From(wrapped)
	if got.Code != CodeNotFound {
		t.Fatalf("From().Code = %v, want %v", got.Code, CodeNotFound)
	}
}

func TestFromFallsBackToInternalForPlainErrors(t *testing.T) {
	got := From(errors.New("boom"))
	if got.Code != CodeInternal {
		t.Fatalf("From().Code = %v, want %v", got.Code, CodeInternal)
	}
}

func TestWrapPreservesUnwrapChain(t *testing.T) {
	sentinel := errors.New("db connection refused")
	wrapped := Wrap(CodeDatabase, sentinel, "")

	if !errors.Is(wrapped, sentinel) {
		t.Fatal("expected errors.Is to see through the wrapped *Error")
	}
}

func TestIsChecksCode(t *testing.T) {
	err := NotFound("product not found")
	if !Is(err, CodeNotFound) {
		t.Fatal("Is() should return true for a matching code")
	}
	if Is(err, CodeForbidden) {
		t.Fatal("Is() should return false for a non-matching code")
	}
}

func TestHTTPStatusMapping(t *testing.T) {
	cases := map[Code]int{
		CodeInvalidParams:   http.StatusBadRequest,
		CodeUnauthorized:    http.StatusUnauthorized,
		CodeNotFound:        http.StatusNotFound,
		CodeDatabase:        http.StatusInternalServerError,
		CodeRequestTooLarge: http.StatusRequestEntityTooLarge,
	}
	for code, want := range cases {
		if got := code.HTTPStatus(); got != want {
			t.Errorf("Code(%d).HTTPStatus() = %d, want %d", code, got, want)
		}
	}
}

// TestRequestTooLarge 验证 RequestTooLarge 的错误码和 HTTP 413 映射。
func TestRequestTooLarge(t *testing.T) {
	e := RequestTooLarge("请求体过大")
	if e.Code != CodeRequestTooLarge {
		t.Errorf("Code = %d, want %d", e.Code, CodeRequestTooLarge)
	}
	if e.Code.HTTPStatus() != http.StatusRequestEntityTooLarge {
		t.Errorf("HTTPStatus() = %d, want 413", e.Code.HTTPStatus())
	}
}

func TestRegisterHTTPStatusForCustomCode(t *testing.T) {
	const customCode Code = 10001
	RegisterHTTPStatus(customCode, http.StatusTeapot)

	if got := customCode.HTTPStatus(); got != http.StatusTeapot {
		t.Fatalf("customCode.HTTPStatus() = %d, want %d", got, http.StatusTeapot)
	}
}

// TestFromHandlesTypedNilWithoutPanicking 验证 From/Is 处理类型化 nil *Error 时不 panic。
func TestFromHandlesTypedNilWithoutPanicking(t *testing.T) {
	var typedNilErr error = (*Error)(nil) // 手动构造陷阱场景，不依赖 Wrap 的具体实现细节

	got := From(typedNilErr)
	if got == nil {
		t.Fatal("From() should never return nil for a non-nil error interface")
	}
	if got.Code != CodeInternal {
		t.Fatalf("From().Code = %v, want %v", got.Code, CodeInternal)
	}

	if Is(typedNilErr, CodeInternal) {
		t.Fatal("Is() should not match a typed-nil *Error against any code")
	}
}

// TestWrapReturnsNilForNilError 验证 Wrap(code, nil, "") 返回 nil。
func TestWrapReturnsNilForNilError(t *testing.T) {
	if got := Wrap(CodeDatabase, nil, ""); got != nil {
		t.Fatalf("Wrap(code, nil, \"\") = %v, want nil", got)
	}
}

func TestNewUsesDefaultMessageWhenEmpty(t *testing.T) {
	err := New(CodeUnauthorized, "")
	if err.Message == "" {
		t.Fatal("expected default message to be filled in")
	}
}

// TestCallerPointsToActualCallSiteNotWrapperFunctions 验证 Caller 指向业务调用点，而不是 errs 内部包装函数。
func TestCallerPointsToActualCallSiteNotWrapperFunctions(t *testing.T) {
	const thisFile = "errs_test.go"

	cases := []*Error{
		New(CodeInternal, "via New"),
		InvalidParams("via InvalidParams"),    // New 的语法糖
		Database(errors.New("db down")),       // Wrap 的语法糖
		Newf(CodeInternal, "via Newf: %d", 1), // Newf 调 New，多一层包装
		NotFound("via NotFound"),              // 语法糖
	}

	for _, err := range cases {
		if !strings.Contains(err.Caller, thisFile) {
			t.Errorf("Caller = %q，应该指向本测试文件（%s），而不是 errs 包内部", err.Caller, thisFile)
		}
	}
}

// BenchmarkCapturedCaller 测量构造错误时捕获调用栈的开销。
func BenchmarkCapturedCaller(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = New(CodeInternal, "benchmark")
	}
}

// BenchmarkHTTPStatus 测量 HTTPStatus 读路径开销。
func BenchmarkHTTPStatus(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = CodeNotFound.HTTPStatus()
		}
	})
}

// TestConcurrentRegisterAndRead 验证并发注册与读取错误码不会 data race。
func TestConcurrentRegisterAndRead(t *testing.T) {
	const customCode Code = 20001
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			RegisterHTTPStatus(customCode, http.StatusBadRequest)
			RegisterMessage(customCode, "custom message")
		}()
		go func() {
			defer wg.Done()
			_ = customCode.HTTPStatus()
			_ = New(customCode, "")
			_ = Wrap(CodeDatabase, errors.New("boom"), "")
		}()
	}
	wg.Wait()
}
