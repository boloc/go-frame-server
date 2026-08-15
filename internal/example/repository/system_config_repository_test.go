package repository

import (
	"context"
	"testing"
)

// TestSystemConfigRepositoryGetValuePanicsWhenDBNotRegistered 验证未注册 config_db 时 GetValue 会 panic。
func TestSystemConfigRepositoryGetValuePanicsWhenDBNotRegistered(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected GetValue() to panic when config_db is not registered")
		}
	}()
	repo := NewSystemConfigRepository()
	_, _ = repo.GetValue(context.Background(), "maintenance_mode")
}

// TestSystemConfigRepositorySetValuePanicsWhenDBNotRegistered 验证未注册 config_db 时 SetValue 会 panic。
func TestSystemConfigRepositorySetValuePanicsWhenDBNotRegistered(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected SetValue() to panic when config_db is not registered")
		}
	}()
	repo := NewSystemConfigRepository()
	_ = repo.SetValue(context.Background(), "maintenance_mode", "on")
}
