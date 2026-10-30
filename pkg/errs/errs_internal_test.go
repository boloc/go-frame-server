package errs

import "testing"

func TestShortFuncName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in, want string
	}{
		{"github.com/a/b/v2/internal/repository.(*SecurityQuestionRepository).ListEnabled", "repository.(*SecurityQuestionRepository).ListEnabled"},
		{"github.com/a/b/handler.ProductList.func1", "handler.ProductList.func1"},
		{"main.main", "main.main"},
		{"github.com/a/b/repo.Find[go.shape.*github.com/a/b/model.User]", "repo.Find[go.shape.*github.com/a/b/model.User]"},
	}
	for _, tt := range tests {
		if got := shortFuncName(tt.in); got != tt.want {
			t.Errorf("shortFuncName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
