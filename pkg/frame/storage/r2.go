package storage

import (
	"context"
	"fmt"
)

// R2Config 是 Cloudflare R2 的简化配置：只需要账户 ID、密钥、桶名——Endpoint（R2 固定的
// 端点域名模式）、UsePathStyle（R2 要求路径风格）这些 S3 兼容层的细节由 NewR2Client
// 按 R2 的实际情况自动填好，不需要调用方了解 S3Config 里那些通用字段的含义。
type R2Config struct {
	AccountID       string
	AccessKeyID     string
	AccessKeySecret string
	BucketName      string
	// Region 留空默认 "auto"，R2 通常不校验这个值。
	Region string
	// PublicBaseURL 建议配置成 R2 的自定义域名；留空会退回 R2 的 S3 API 端点拼出来的
	// 地址，那不是一个默认公开可访问的 URL（除非单独给桶配置了公开访问）。
	PublicBaseURL string
}

// NewR2Client 创建一个连接 Cloudflare R2 的 S3Client。R2 的 API 跟 S3 兼容，这里只是
// 把 R2 特有的默认值（Endpoint、UsePathStyle）填好后委托给 NewS3Client。
func NewR2Client(ctx context.Context, cfg R2Config) (*S3Client, error) {
	if cfg.AccountID == "" {
		return nil, fmt.Errorf("storage: 缺少 AccountID")
	}
	region := cfg.Region
	if region == "" {
		region = "auto"
	}

	return NewS3Client(ctx, S3Config{
		AccessKeyID:     cfg.AccessKeyID,
		AccessKeySecret: cfg.AccessKeySecret,
		BucketName:      cfg.BucketName,
		Region:          region,
		Endpoint:        fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.AccountID),
		UsePathStyle:    true,
		PublicBaseURL:   cfg.PublicBaseURL,
	})
}
