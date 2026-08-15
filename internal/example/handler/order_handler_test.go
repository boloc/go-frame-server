package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// newTestRouter 只挂订单路由；仓储是内存实现，不依赖真实 MySQL/Redis。
func newTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/orders", OrderCreate)
	return r
}

func postOrder(t *testing.T, r *gin.Engine, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/orders", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response body: %v, raw=%s", err, w.Body.String())
	}
	return out
}

func TestOrderCreate_Success(t *testing.T) {
	r := newTestRouter()
	w := postOrder(t, r, map[string]any{"product_id": 1, "quantity": 2})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	body := decodeBody(t, w)
	if body["code"].(float64) != 0 {
		t.Fatalf("code = %v, want 0", body["code"])
	}
	data := body["data"].(map[string]any)
	if data["status"] != "created" {
		t.Fatalf("order status = %v, want created", data["status"])
	}
}

// 失败场景走 webx.Fail，HTTP 状态码为 200，错误看 body.code。

func TestOrderCreate_ValidationError(t *testing.T) {
	r := newTestRouter()
	// quantity=0 违反 validate:"required,min=1"。
	w := postOrder(t, r, map[string]any{"product_id": 1, "quantity": 0})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (webx.Fail default), body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	body := decodeBody(t, w)
	if body["code"].(float64) != 40000 { // errs.CodeInvalidParams
		t.Fatalf("code = %v, want 40000", body["code"])
	}
	if _, ok := body["fields"]; !ok {
		t.Fatal("expected field-level validation details in response")
	}
}

func TestOrderCreate_BannedProductRejectedByValidationLayer(t *testing.T) {
	r := newTestRouter()
	// 产品 4 号在 validation 层被标为下架，不查库。
	w := postOrder(t, r, map[string]any{"product_id": 4, "quantity": 1})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (webx.Fail default), body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	body := decodeBody(t, w)
	if body["code"].(float64) != 40000 { // errs.CodeInvalidParams
		t.Fatalf("code = %v, want 40000", body["code"])
	}
}

func TestOrderCreate_InsufficientStock(t *testing.T) {
	r := newTestRouter()
	// 产品 2 号库存为 0，触发 bizerr.CodeInsufficientStock。
	w := postOrder(t, r, map[string]any{"product_id": 2, "quantity": 1})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (webx.Fail default), body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	body := decodeBody(t, w)
	if body["code"].(float64) != 10002 { // bizerr.CodeInsufficientStock
		t.Fatalf("code = %v, want 10002", body["code"])
	}
}

func TestOrderCreate_ProductNotFound(t *testing.T) {
	r := newTestRouter()
	w := postOrder(t, r, map[string]any{"product_id": 3, "quantity": 1})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (webx.Fail default), body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	body := decodeBody(t, w)
	if body["code"].(float64) != 40400 { // errs.CodeNotFound
		t.Fatalf("code = %v, want 40400", body["code"])
	}
}

func TestOrderCreate_SimulatedDatabaseFailure(t *testing.T) {
	r := newTestRouter()
	w := postOrder(t, r, map[string]any{"product_id": 999, "quantity": 1})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (webx.Fail default), body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	body := decodeBody(t, w)
	if body["code"].(float64) != 50001 { // errs.CodeDatabase
		t.Fatalf("code = %v, want 50001", body["code"])
	}
}
