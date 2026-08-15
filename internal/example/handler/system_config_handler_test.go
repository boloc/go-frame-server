package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestSystemConfigGetByKeyPanicsAsHTTP500WhenNotConnected 验证未注册 config_db 时 panic 被 Recovery 成 HTTP 500。
func TestSystemConfigGetByKeyPanicsAsHTTP500WhenNotConnected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/api/system-configs/:key", SystemConfigGetByKey)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/system-configs/maintenance_mode", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d（gin.Recovery() 恢复 panic 后的默认状态码）", w.Code, http.StatusInternalServerError)
	}
}

// TestSystemConfigSetPanicsAsHTTP500WhenNotConnected 验证写路径同样在未注册时 panic 成 HTTP 500。
func TestSystemConfigSetPanicsAsHTTP500WhenNotConnected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.POST("/api/system-configs/:key", SystemConfigSet)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/system-configs/maintenance_mode",
		strings.NewReader(`{"value":"on"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d（gin.Recovery() 恢复 panic 后的默认状态码）", w.Code, http.StatusInternalServerError)
	}
}
