package components

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestNewGinComponentDefaultsAreProductionSafe(t *testing.T) {
	g := NewGinComponent()

	// 超时和头大小默认值必须 > 0，TrustedProxies 默认不信任任何代理。
	if g.config.ReadHeaderTimeout <= 0 {
		t.Error("ReadHeaderTimeout default must be > 0")
	}
	if g.config.ReadTimeout <= 0 {
		t.Error("ReadTimeout default must be > 0")
	}
	if g.config.WriteTimeout <= 0 {
		t.Error("WriteTimeout default must be > 0")
	}
	if g.config.IdleTimeout <= 0 {
		t.Error("IdleTimeout default must be > 0")
	}
	if g.config.MaxHeaderBytes <= 0 {
		t.Error("MaxHeaderBytes default must be > 0")
	}
	if g.config.TrustedProxies != nil {
		t.Errorf("TrustedProxies default must be nil (trust nothing), got %v", g.config.TrustedProxies)
	}
}

func TestGinOptionsOverrideDefaults(t *testing.T) {
	g := NewGinComponent(
		WithGinReadHeaderTimeout(1*time.Second),
		WithGinTrustedProxies([]string{"10.0.0.0/8"}),
	)

	if g.config.ReadHeaderTimeout != 1*time.Second {
		t.Errorf("ReadHeaderTimeout = %v, want 1s", g.config.ReadHeaderTimeout)
	}
	// 未覆盖字段应保留安全默认值。
	if g.config.ReadTimeout <= 0 {
		t.Error("ReadTimeout should still have its default value")
	}
	if len(g.config.TrustedProxies) != 1 || g.config.TrustedProxies[0] != "10.0.0.0/8" {
		t.Errorf("TrustedProxies = %v, want [10.0.0.0/8]", g.config.TrustedProxies)
	}
}

// TestClientIPWithDefaultTrustedProxies 验证默认不信任代理，ClientIP 取真实对端而非伪造 XFF。
func TestClientIPWithDefaultTrustedProxies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g := NewGinComponent() // 默认 TrustedProxies = nil
	engine := g.GetEngine()
	if err := engine.SetTrustedProxies(g.config.TrustedProxies); err != nil {
		t.Fatalf("SetTrustedProxies: %v", err)
	}

	var gotIP string
	engine.GET("/ip", func(c *gin.Context) { gotIP = c.ClientIP() })

	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "1.2.3.4:54321" // 模拟直连场景下 TCP 层面的真实客户端地址
	req.Header.Set("X-Forwarded-For", "9.9.9.9")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if gotIP != "1.2.3.4" {
		t.Fatalf("ClientIP() = %q, want %q（说明伪造的 X-Forwarded-For 被采信了，TrustedProxies 没生效）", gotIP, "1.2.3.4")
	}
}

// TestClientIPBehindConfiguredProxy 验证配置 TrustedProxies 后从 XFF 取真实用户 IP。
func TestClientIPBehindConfiguredProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g := NewGinComponent(WithGinTrustedProxies([]string{"10.0.0.0/8"}))
	engine := g.GetEngine()
	if err := engine.SetTrustedProxies(g.config.TrustedProxies); err != nil {
		t.Fatalf("SetTrustedProxies: %v", err)
	}

	var gotIP string
	engine.GET("/ip", func(c *gin.Context) { gotIP = c.ClientIP() })

	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "10.1.2.3:54321" // 网关的出口 IP，在 10.0.0.0/8 段内，会被信任
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if gotIP != "203.0.113.7" {
		t.Fatalf("ClientIP() = %q, want %q（说明配置了 TrustedProxies 之后没能从 X-Forwarded-For 拿到真实用户 IP）", gotIP, "203.0.113.7")
	}
}

// TestClientIPBehindUnconfiguredProxy 验证未配置 TrustedProxies 时请求仍成功，ClientIP 为代理 IP。
func TestClientIPBehindUnconfiguredProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g := NewGinComponent() // 默认 TrustedProxies = nil，但请求实际上经过了代理
	engine := g.GetEngine()
	if err := engine.SetTrustedProxies(g.config.TrustedProxies); err != nil {
		t.Fatalf("SetTrustedProxies: %v", err)
	}

	var gotIP string
	engine.GET("/ip", func(c *gin.Context) { gotIP = c.ClientIP() })

	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "10.1.2.3:54321" // 代理的出口 IP
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200（TrustedProxies 配置不当不应该导致请求被拒绝）", w.Code)
	}
	if gotIP != "10.1.2.3" {
		t.Fatalf("ClientIP() = %q, want %q", gotIP, "10.1.2.3")
	}
}

// TestDetectUnconfiguredProxyWarnsOnce 验证未配置 TrustedProxies 却收到转发头时只警告一次。
func TestDetectUnconfiguredProxyWarnsOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g := NewGinComponent() // TrustedProxies 未配置，detectUnconfiguredProxy 中间件会被自动挂上
	engine := g.GetEngine()
	engine.GET("/ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })

	if g.warnedForwardedHeader.Load() {
		t.Fatal("尚未收到任何请求，不应该已经警告过")
	}

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/ip", nil)
		req.Header.Set("X-Forwarded-For", "203.0.113.7")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
	}

	if !g.warnedForwardedHeader.Load() {
		t.Fatal("收到带转发头的请求后应该已经标记为警告过")
	}
}

// TestDetectUnconfiguredProxyNotTriggeredWhenConfigured 验证已配置 TrustedProxies 时不触发警告。
func TestDetectUnconfiguredProxyNotTriggeredWhenConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g := NewGinComponent(WithGinTrustedProxies([]string{"10.0.0.0/8"}))
	engine := g.GetEngine()
	engine.GET("/ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })

	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if g.warnedForwardedHeader.Load() {
		t.Fatal("TrustedProxies 已配置时不应该触发这条警告标记")
	}
}

func TestGinComponentStartStop(t *testing.T) {
	// Port "0" 让操作系统分配空闲端口。
	g := NewGinComponent(WithGinPort("0"), WithGinShutdownTimeout(2*time.Second))

	if err := g.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	if err := g.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

// TestStartFailsSynchronouslyWhenPortInUse 验证端口被占用时 Start 同步返回 error。
func TestStartFailsSynchronouslyWhenPortInUse(t *testing.T) {
	// 先占住一个空闲端口，再让 GinComponent 绑定同一端口。
	occupier, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to occupy a port: %v", err)
	}
	defer occupier.Close()

	port := strconv.Itoa(occupier.Addr().(*net.TCPAddr).Port)

	g := NewGinComponent(WithGinPort(port))
	if err := g.Start(context.Background()); err == nil {
		g.Stop(context.Background())
		t.Fatal("expected Start() to fail synchronously when the port is already in use")
	}
}

func TestNewGinComponentBodyLimitDefaultIsSet(t *testing.T) {
	g := NewGinComponent()
	if g.config.MaxBodyBytes <= 0 {
		t.Error("MaxBodyBytes default must be > 0")
	}
	if g.config.RequestTimeout != 0 {
		t.Errorf("RequestTimeout default must be 0 (disabled), got %v", g.config.RequestTimeout)
	}
}

// TestMaxBodyBytesOptionRejectsOversizedBody 验证超限请求体返回 413，而不是截断后继续处理。
func TestMaxBodyBytesOptionRejectsOversizedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g := NewGinComponent(WithGinMaxBodyBytes(8))
	engine := g.GetEngine()
	engine.POST("/echo", func(c *gin.Context) {
		body, err := c.GetRawData()
		if err != nil {
			c.String(http.StatusRequestEntityTooLarge, "too large")
			return
		}
		c.String(http.StatusOK, string(body))
	})

	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader("this body is way over 8 bytes"))
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413（超过 MaxBodyBytes 限制的请求体应该被拒绝）", w.Code)
	}
}

// TestMaxBodyBytesOptionAllowsBodyWithinLimit 验证未超限的请求体正常放行。
func TestMaxBodyBytesOptionAllowsBodyWithinLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g := NewGinComponent(WithGinMaxBodyBytes(1024))
	engine := g.GetEngine()
	engine.POST("/echo", func(c *gin.Context) {
		body, err := c.GetRawData()
		if err != nil {
			c.String(http.StatusRequestEntityTooLarge, "too large")
			return
		}
		c.String(http.StatusOK, string(body))
	})

	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader("small body"))
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK || w.Body.String() != "small body" {
		t.Fatalf("status=%d body=%q, want 200 and echoed body", w.Code, w.Body.String())
	}
}

// TestGinComponentSurvivesRestartWithoutReregisteringRoutes 模拟 Frame.Restart
// （Stop 再 Start）：routerRegistrar 不能被执行第二次，否则同一个 g.engine 上重复
// 注册路径会让 gin panic。验证重启后路由仍能正常响应。
func TestGinComponentSurvivesRestartWithoutReregisteringRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registerCalls := 0
	g := NewGinComponent(WithGinPort("0"), WithGinShutdownTimeout(2*time.Second))
	g.routerRegistrar = func(e *gin.Engine) {
		registerCalls++
		e.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })
	}

	if err := g.Start(context.Background()); err != nil {
		t.Fatalf("first Start() error = %v", err)
	}
	if err := g.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	// 第二次 Start 模拟 Restart；这里不能 panic。
	if err := g.Start(context.Background()); err != nil {
		t.Fatalf("second Start() (simulated restart) error = %v", err)
	}
	defer g.Stop(context.Background())

	if registerCalls != 1 {
		t.Fatalf("routerRegistrar 应该只被执行一次，实际执行了 %d 次", registerCalls)
	}

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	g.engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "pong" {
		t.Fatalf("重启之后路由应该仍然能正常响应，got status=%d body=%q", w.Code, w.Body.String())
	}
}

// TestRequestTimeoutOptionCancelsContext 验证 RequestTimeout 到期后会取消请求 ctx。
func TestRequestTimeoutOptionCancelsContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g := NewGinComponent(WithGinRequestTimeout(20 * time.Millisecond))
	engine := g.GetEngine()

	var ctxErrAfterWait error
	engine.GET("/slow", func(c *gin.Context) {
		<-c.Request.Context().Done()
		ctxErrAfterWait = c.Request.Context().Err()
		c.String(http.StatusOK, "done")
	})

	req := httptest.NewRequest(http.MethodGet, "/slow", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if ctxErrAfterWait == nil {
		t.Fatal("expected c.Request.Context() to be cancelled after RequestTimeout elapses")
	}
}
