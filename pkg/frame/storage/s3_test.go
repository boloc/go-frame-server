package storage

import (
	"context"
	"testing"
)

// TestTruncateFileNameKeepsExtensionWithoutPanicking 覆盖之前的 bug：文件名扩展名本身
// 长度就达到甚至超过 maxLen 时，截断计算会得到负数，对字符串取负数下标直接 panic。
func TestTruncateFileNameKeepsExtensionWithoutPanicking(t *testing.T) {
	cases := []struct {
		name    string
		fname   string
		ext     string
		maxLen  int
		wantLen int // 只断言不超过这个长度，不断言具体截断出的字符串
	}{
		{"正常截断", "a-very-long-file-name.jpg", ".jpg", 20, 20},
		{"扩展名本身就等于 maxLen", "file.verylongext", ".verylongext", 12, 12},
		{"扩展名比 maxLen 还长", "file.evenlongerextension", ".evenlongerextension", 5, 20},
		{"不需要截断", "short.jpg", ".jpg", 20, 9},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateFileName(tc.fname, tc.ext, tc.maxLen)
			if len(got) > tc.wantLen {
				t.Fatalf("truncateFileName(%q, %q, %d) = %q（长度 %d），超过预期上限 %d",
					tc.fname, tc.ext, tc.maxLen, got, len(got), tc.wantLen)
			}
		})
	}
}

// TestNewS3ClientRejectsIncompleteConfig 验证缺少必填字段时返回 error，不 panic。
func TestNewS3ClientRejectsIncompleteConfig(t *testing.T) {
	_, err := NewS3Client(context.Background(), S3Config{})
	if err == nil {
		t.Fatal("缺少必填配置时 NewS3Client 应该返回 error")
	}
}

// TestNewS3ClientDoesNotMutateCallerConfig 验证 S3Config 按值传递，调用方持有的原始
// 配置不会被 NewS3Client 内部逻辑意外修改。
func TestNewS3ClientDoesNotMutateCallerConfig(t *testing.T) {
	cfg := S3Config{
		AccessKeyID:     "id",
		AccessKeySecret: "secret",
		BucketName:      "bucket",
		Region:          "", // 故意留空，验证不会被就地改写成别的值
	}
	_, _ = NewS3Client(context.Background(), cfg)

	if cfg.Region != "" {
		t.Fatalf("cfg.Region 被意外修改成了 %q，S3Config 应该是按值传递、不可被内部逻辑影响调用方持有的原始配置", cfg.Region)
	}
}

// TestNewR2ClientRejectsMissingAccountID 验证缺少 AccountID 时返回 error。
func TestNewR2ClientRejectsMissingAccountID(t *testing.T) {
	_, err := NewR2Client(context.Background(), R2Config{
		AccessKeyID: "id", AccessKeySecret: "secret", BucketName: "bucket",
	})
	if err == nil {
		t.Fatal("缺少 AccountID 时 NewR2Client 应该返回 error")
	}
}

// TestPublicURLPrefersPublicBaseURLOverEndpointFallback 验证配置了 PublicBaseURL 时
// 优先用它拼 URL，而不是退回 Endpoint。
func TestPublicURLPrefersPublicBaseURLOverEndpointFallback(t *testing.T) {
	c := &S3Client{
		bucketName:    "bucket",
		endpoint:      "https://example.r2.cloudflarestorage.com",
		publicBaseURL: "https://cdn.example.com",
	}
	got := c.publicURL("uploads/a.jpg")
	want := "https://cdn.example.com/uploads/a.jpg"
	if got != want {
		t.Fatalf("publicURL() = %q, want %q", got, want)
	}
}

// TestPublicURLFallsBackToEndpointWhenPublicBaseURLEmpty 验证没配 PublicBaseURL 时退回
// Endpoint 拼出来的地址。
func TestPublicURLFallsBackToEndpointWhenPublicBaseURLEmpty(t *testing.T) {
	c := &S3Client{
		bucketName: "bucket",
		endpoint:   "https://example.r2.cloudflarestorage.com",
	}
	got := c.publicURL("uploads/a.jpg")
	want := "https://example.r2.cloudflarestorage.com/bucket/uploads/a.jpg"
	if got != want {
		t.Fatalf("publicURL() = %q, want %q", got, want)
	}
}
