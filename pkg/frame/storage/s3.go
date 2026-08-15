// Package storage 提供 S3 兼容对象存储的上传/下载/管理能力。真正的 AWS S3、
// Cloudflare R2、MinIO、Backblaze B2 等任何实现了 S3 API 的服务都可以用 S3Client
// 接入——Cloudflare R2 有专门的 NewR2Client 便捷构造函数（见 r2.go），其它服务
// 直接用 NewS3Client 显式填好 Endpoint/UsePathStyle。
package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
)

const defaultContentType = "application/octet-stream"

// S3Config 连接一个 S3 兼容对象存储服务需要的配置。
type S3Config struct {
	AccessKeyID     string
	AccessKeySecret string
	BucketName      string
	// Region 真正的 AWS S3 必须填真实区域（如 "us-east-1"）；S3 兼容服务通常随便填一个
	// 非空值即可（Cloudflare R2 习惯用 "auto"）。
	Region string
	// Endpoint 留空表示用 AWS SDK 按 Region 解析出的官方 S3 端点（连真正的 AWS S3 用）；
	// 接入 S3 兼容服务（R2/MinIO/...）必须显式填这家服务自己的端点。
	Endpoint string
	// UsePathStyle 真正的 AWS S3 默认 false（虚拟主机风格）；大多数 S3 兼容服务需要 true
	//（路径风格），具体看对方文档。
	UsePathStyle bool
	// PublicBaseURL 拼接公开访问 URL 时用的域名前缀（自定义域名/CDN）。留空时 PublicURL
	// 会退回用 Endpoint 拼出来的地址——那通常是这家服务的 API 端点，不一定是能公开
	// 直接访问的 URL（取决于服务商和桶的公开读配置），生产环境建议总是显式配置。
	PublicBaseURL string
}

// S3Client 是对 S3 兼容对象存储的上传/下载/管理封装。
type S3Client struct {
	client        *s3.Client
	bucketName    string
	endpoint      string
	publicBaseURL string
}

// UploadResult 是一次上传的结果。
type UploadResult struct {
	URL         string
	Key         string
	Size        int64
	ContentType string
}

// NewS3Client 创建一个 S3 兼容对象存储客户端。
func NewS3Client(ctx context.Context, cfg S3Config) (*S3Client, error) {
	if cfg.AccessKeyID == "" || cfg.AccessKeySecret == "" || cfg.BucketName == "" {
		return nil, fmt.Errorf("storage: 缺少必要的配置（AccessKeyID/AccessKeySecret/BucketName）")
	}

	sdkConfig, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID, cfg.AccessKeySecret, "",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("storage: 加载 AWS SDK 配置失败: %w", err)
	}

	client := s3.NewFromConfig(sdkConfig, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = cfg.UsePathStyle
	})

	return &S3Client{
		client:        client,
		bucketName:    cfg.BucketName,
		endpoint:      cfg.Endpoint,
		publicBaseURL: strings.TrimRight(cfg.PublicBaseURL, "/"),
	}, nil
}

// UploadFile 从本地文件路径上传。
func (c *S3Client) UploadFile(ctx context.Context, filePath string, objectKey string, contentType string) (*UploadResult, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("storage: 打开文件失败: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("storage: 获取文件信息失败: %w", err)
	}

	if objectKey == "" {
		objectKey = uuid.New().String() + filepath.Ext(filePath)
	}
	if contentType == "" {
		contentType = detectContentType(filePath)
	}

	return c.UploadData(ctx, file, objectKey, contentType, fileInfo.Size())
}

// UploadBytes 上传字节数组。
func (c *S3Client) UploadBytes(ctx context.Context, data []byte, objectKey string, contentType string) (*UploadResult, error) {
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	return c.UploadData(ctx, bytes.NewReader(data), objectKey, contentType, int64(len(data)))
}

// UploadData 从 io.Reader 上传数据。
func (c *S3Client) UploadData(ctx context.Context, reader io.Reader, objectKey string, contentType string, size int64) (*UploadResult, error) {
	if contentType == "" {
		contentType = defaultContentType
	}
	if objectKey == "" {
		objectKey = uuid.New().String()
	}

	_, err := c.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(c.bucketName),
		Key:           aws.String(objectKey),
		Body:          reader,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
	})
	if err != nil {
		return nil, fmt.Errorf("storage: 上传失败: %w", err)
	}

	return &UploadResult{
		URL:         c.publicURL(objectKey),
		Key:         objectKey,
		Size:        size,
		ContentType: contentType,
	}, nil
}

// UploadMultipartFile 上传一个 HTTP multipart 文件。
func (c *S3Client) UploadMultipartFile(ctx context.Context, file *multipart.FileHeader, objectKey string, contentType string) (*UploadResult, error) {
	srcFile, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("storage: 打开上传文件失败: %w", err)
	}
	defer srcFile.Close()

	ext := filepath.Ext(file.Filename)
	if objectKey == "" {
		fileName := truncateFileName(filepath.Base(file.Filename), ext, 20)
		objectKey = fmt.Sprintf("uploads/%s-%s-%s", time.Now().Format("20060102"), uuid.New().String()[:8], fileName)
	}

	if contentType == "" {
		contentType = file.Header.Get("Content-Type")
		if contentType == "" {
			contentType = contentTypeFromExt(ext)
		}
	}

	return c.UploadData(ctx, srcFile, objectKey, contentType, file.Size)
}

// truncateFileName 把 fileName 截断到最多 maxLen 个字符，保留扩展名 ext。
// ext 本身达到或超过 maxLen 时，只保留 ext，不截断出负长度导致 panic。
func truncateFileName(fileName, ext string, maxLen int) string {
	if len(fileName) <= maxLen {
		return fileName
	}
	withoutExt := strings.TrimSuffix(fileName, ext)
	keep := maxLen - len(ext)
	if keep < 0 {
		keep = 0
	}
	if keep > len(withoutExt) {
		keep = len(withoutExt)
	}
	return withoutExt[:keep] + ext
}

// UploadReader 从任意 io.Reader 上传；不是 io.ReadSeeker 时会先整体读入内存。
func (c *S3Client) UploadReader(ctx context.Context, reader io.Reader, objectKey string, contentType string, size int64) (*UploadResult, error) {
	readSeeker, ok := reader.(io.ReadSeeker)
	if !ok {
		data, err := io.ReadAll(reader)
		if err != nil {
			return nil, fmt.Errorf("storage: 读取内容到内存失败: %w", err)
		}
		readSeeker = bytes.NewReader(data)
		if size <= 0 {
			size = int64(len(data))
		}
	}
	return c.UploadData(ctx, readSeeker, objectKey, contentType, size)
}

// UploadAny 是一个通用上传方法，支持文件路径(string)、[]byte、io.Reader、
// *multipart.FileHeader；类型不对时返回 error。类型明确时优先用对应的 UploadXxx 方法，
// 能拿到编译期类型检查。
func (c *S3Client) UploadAny(ctx context.Context, source any, objectKey string, contentType string) (*UploadResult, error) {
	switch src := source.(type) {
	case string:
		return c.UploadFile(ctx, src, objectKey, contentType)
	case []byte:
		return c.UploadBytes(ctx, src, objectKey, contentType)
	case *multipart.FileHeader:
		return c.UploadMultipartFile(ctx, src, objectKey, contentType)
	case io.Reader:
		return c.UploadReader(ctx, src, objectKey, contentType, -1)
	default:
		return nil, fmt.Errorf("storage: 不支持的上传源类型: %T", source)
	}
}

// GeneratePresignedURL 生成一个限时访问的预签名 URL。
func (c *S3Client) GeneratePresignedURL(ctx context.Context, objectKey string, expires time.Duration) (string, error) {
	presigner := s3.NewPresignClient(c.client)
	request, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(objectKey),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expires
	})
	if err != nil {
		return "", fmt.Errorf("storage: 生成预签名 URL 失败: %w", err)
	}
	return request.URL, nil
}

// DeleteObject 删除一个对象。
func (c *S3Client) DeleteObject(ctx context.Context, objectKey string) error {
	if _, err := c.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(objectKey),
	}); err != nil {
		return fmt.Errorf("storage: 删除对象失败: %w", err)
	}
	return nil
}

// ListObjects 按前缀列出对象键。maxKeys <= 0 时使用 S3 API 的默认上限。
func (c *S3Client) ListObjects(ctx context.Context, prefix string, maxKeys int32) ([]string, error) {
	input := &s3.ListObjectsV2Input{Bucket: aws.String(c.bucketName)}
	if prefix != "" {
		input.Prefix = aws.String(prefix)
	}
	if maxKeys > 0 {
		input.MaxKeys = aws.Int32(maxKeys)
	}

	result, err := c.client.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("storage: 列出对象失败: %w", err)
	}

	keys := make([]string, 0, len(result.Contents))
	for _, obj := range result.Contents {
		keys = append(keys, *obj.Key)
	}
	return keys, nil
}

// publicURL 拼接 objectKey 的公开访问地址，见 S3Config.PublicBaseURL 的文档注释。
func (c *S3Client) publicURL(objectKey string) string {
	if c.publicBaseURL != "" {
		return c.publicBaseURL + "/" + objectKey
	}
	return strings.TrimRight(c.endpoint, "/") + "/" + c.bucketName + "/" + objectKey
}

func contentTypeFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".pdf":
		return "application/pdf"
	case ".doc", ".docx":
		return "application/msword"
	case ".xls", ".xlsx":
		return "application/vnd.ms-excel"
	default:
		return defaultContentType
	}
}

// detectContentType 读取文件头 512 字节检测内容类型。
func detectContentType(filePath string) string {
	file, err := os.Open(filePath)
	if err != nil {
		return defaultContentType
	}
	defer file.Close()

	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return defaultContentType
	}
	return http.DetectContentType(buffer[:n])
}
