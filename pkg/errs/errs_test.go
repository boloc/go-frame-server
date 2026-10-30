package errs_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/boloc/go-frame-server/v2/pkg/errs"
)

type questionRepo struct{}

func (*questionRepo) ListEnabled() error {
	return errs.Database(errors.New("boom"))
}

func TestFuncAndCaller(t *testing.T) {
	t.Parallel()

	e := errs.From((&questionRepo{}).ListEnabled())
	if want := "errs_test.(*questionRepo).ListEnabled"; e.Func != want {
		t.Fatalf("Func = %q, want %q", e.Func, want)
	}
	if !strings.Contains(e.Caller, "errs_test.go:") {
		t.Fatalf("Caller = %q, want errs_test.go:<line>", e.Caller)
	}

	n := errs.NotFound("x")
	if want := "errs_test.TestFuncAndCaller"; n.Func != want {
		t.Fatalf("New Func = %q, want %q", n.Func, want)
	}
}

type registerLogic struct{ repo *questionRepo }

func (l *registerLogic) Register() error {
	if err := l.repo.ListEnabled(); err != nil {
		return errs.Wrap(errs.CodeInternal, fmt.Errorf("load questions: %w", err), "注册失败")
	}
	return nil
}

func TestWrapKeepsOrigin(t *testing.T) {
	t.Parallel()

	e := errs.From((&registerLogic{repo: &questionRepo{}}).Register())
	if e.Code != errs.CodeInternal || e.Message != "注册失败" {
		t.Fatalf("outer code/message = %d %q", e.Code, e.Message)
	}
	if want := "errs_test.(*questionRepo).ListEnabled"; e.Func != want {
		t.Fatalf("Func = %q, want origin %q", e.Func, want)
	}
	if want := "[50000] 注册失败: load questions: [50001] data processing failed: boom"; e.Error() != want {
		t.Fatalf("Error() = %q, want %q", e.Error(), want)
	}

	plain := errs.Internal(errors.New("no origin"))
	if want := "errs_test.TestWrapKeepsOrigin"; plain.Func != want {
		t.Fatalf("plain Func = %q, want %q", plain.Func, want)
	}
}
