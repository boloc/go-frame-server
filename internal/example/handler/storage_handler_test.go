package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestStorageHandlersFailGracefullyWhenNotConfigured 验证没有调用过
// storage.SetDefault（对应"未配置 storage.r2"）时，这几个接口都走 webx.Fail 的
// 正常业务错误响应（HTTP 200，业务码在 body.code），不会 panic。
func TestStorageHandlersFailGracefullyWhenNotConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/upload", StorageUpload)
	r.GET("/objects", StorageList)
	r.DELETE("/objects", StorageDelete)
	r.GET("/presigned-url", StoragePresignedURL)

	cases := []struct {
		name   string
		method string
		url    string
	}{
		{"upload", http.MethodPost, "/upload"},
		{"list", http.MethodGet, "/objects"},
		{"delete", http.MethodDelete, "/objects?key=x"},
		{"presigned-url", http.MethodGet, "/presigned-url?key=x"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.url, nil)
			r.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200（webx.Fail 约定，业务码放在 body.code 里）", w.Code)
			}
			body := decodeBody(t, w)
			if code, _ := body["code"].(float64); code == 0 {
				t.Fatalf("body.code = %v, want 非 0 的业务错误码（对象存储未配置）", body["code"])
			}
		})
	}
}
