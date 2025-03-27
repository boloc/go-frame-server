package client

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

// 默认内容类型
const defaultContentType = "application/octet-stream"

// R2Config 保存 Cloudflare R2 的配置信息
type R2Config struct {
	AccountID       string // 账户ID
	AccessKeyID     string // 访问密钥ID
	AccessKeySecret string // 访问密钥密钥
	BucketName      string // 存储桶名称
	Region          string // 区域
	Endpoint        string // 端点
	CustomDomain    string // 自定义显示用的域名
}

// R2Client 处理 Cloudflare R2 的文件上传
type R2Client struct {
	client     *s3.Client
	bucketName string
	config     *R2Config
}

// UploadResult 包含已上传文件的信息
type UploadResult struct {
	URL         string // 文件访问 URL
	Key         string // 文件键名
	Size        int64  // 文件大小（字节）
	ContentType string // 内容类型
	// LastModified time.Time // 最后修改时间
}

// NewR2Client 使用提供的配置创建一个新的 R2Client
func NewR2Client(cfg *R2Config) (*R2Client, error) {
	if cfg.AccountID == "" || cfg.AccessKeyID == "" || cfg.AccessKeySecret == "" || cfg.BucketName == "" {
		return nil, fmt.Errorf("缺少必要的 R2 配置信息")
	}

	// 如果未提供区域，则设置默认区域
	if cfg.Region == "" {
		cfg.Region = "auto"
	}

	// 为 R2 创建自定义端点 URL
	endpointURL := cfg.Endpoint
	if endpointURL == "" {
		endpointURL = fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.AccountID)
	}

	// 配置 AWS SDK（仅包含基本配置，端点将在 S3 客户端中配置）
	sdkConfig, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID,
			cfg.AccessKeySecret,
			"",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("加载 AWS SDK 配置失败: %w", err)
	}

	// 创建 S3 客户端（R2 使用与 S3 兼容的 API）
	// 使用最新的方法直接在客户端选项中设置端点
	client := s3.NewFromConfig(sdkConfig, func(o *s3.Options) {
		// 设置自定义端点 URL
		o.BaseEndpoint = aws.String(endpointURL)
		// 使用路径样式而不是虚拟主机样式的 URL
		o.UsePathStyle = true
	})

	return &R2Client{
		client:     client,
		bucketName: cfg.BucketName,
		config:     cfg,
	}, nil
}

// UploadFile 从本地文件路径上传文件到 R2
func (r *R2Client) UploadFile(ctx context.Context, filePath string, objectKey string, contentType string) (*UploadResult, error) {
	// 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %w", err)
	}
	defer file.Close()

	// 获取文件信息
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("获取文件信息失败: %w", err)
	}

	// 如果未提供对象键，则使用文件名
	if objectKey == "" {
		// 生成唯一文件名以避免冲突
		ext := filepath.Ext(filePath)
		objectKey = fmt.Sprintf("%s%s", uuid.New().String(), ext)
	}

	// 如果未提供内容类型，尝试检测它
	if contentType == "" {
		contentType = detectContentType(filePath)
	}

	return r.UploadData(ctx, file, objectKey, contentType, fileInfo.Size())
}

// UploadBytes 上传字节数组到 R2
func (r *R2Client) UploadBytes(ctx context.Context, data []byte, objectKey string, contentType string) (*UploadResult, error) {
	reader := bytes.NewReader(data)

	if contentType == "" {
		// 尝试从字节中检测内容类型
		contentType = http.DetectContentType(data)
	}

	return r.UploadData(ctx, reader, objectKey, contentType, int64(len(data)))
}

// UploadData 从 io.Reader 上传数据到 R2
func (r *R2Client) UploadData(ctx context.Context, reader io.Reader, objectKey string, contentType string, size int64) (*UploadResult, error) {
	// 如果未提供内容类型，则使用默认类型
	if contentType == "" {
		contentType = defaultContentType
	}

	// 如果未提供对象键，则生成唯一键
	if objectKey == "" {
		objectKey = uuid.New().String()
	}

	// 上传到 R2
	input := &s3.PutObjectInput{
		Bucket:        aws.String(r.bucketName),
		Key:           aws.String(objectKey),
		Body:          reader,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
	}

	_, err := r.client.PutObject(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("上传到 R2 失败: %w", err)
	}

	// // 获取对象详情以确认上传
	// headInput := &s3.HeadObjectInput{
	// 	Bucket: aws.String(r.bucketName),
	// 	Key:    aws.String(objectKey),
	// }
	// headOutput, err := r.client.HeadObject(ctx, headInput)
	// if err != nil {
	// 	return nil, fmt.Errorf("确认上传失败: %w", err)
	// }

	// 创建公共 URL
	publicURL := fmt.Sprintf("https://%s.r2.cloudflarestorage.com/%s/%s",
		r.config.AccountID, r.bucketName, objectKey)
	if r.config.CustomDomain != "" {
		customDomain := strings.TrimRight(r.config.CustomDomain, "/")
		publicURL = fmt.Sprintf("%s/%s", customDomain, objectKey)
	}

	// 返回结果
	return &UploadResult{
		URL:         publicURL,
		Key:         objectKey,
		Size:        size,
		ContentType: contentType,
		// LastModified: *headOutput.LastModified,
	}, nil
}

// GeneratePresignedURL 为对象生成预签名 URL
func (r *R2Client) GeneratePresignedURL(ctx context.Context, objectKey string, expires time.Duration) (string, error) {
	// 创建 S3 预签名器
	presigner := s3.NewPresignClient(r.client)

	// 创建预签名请求
	request, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(r.bucketName),
		Key:    aws.String(objectKey),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expires
	})

	if err != nil {
		return "", fmt.Errorf("生成预签名 URL 失败: %w", err)
	}

	return request.URL, nil
}

// DeleteObject 从 R2 删除对象
func (r *R2Client) DeleteObject(ctx context.Context, objectKey string) error {
	_, err := r.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(r.bucketName),
		Key:    aws.String(objectKey),
	})

	if err != nil {
		return fmt.Errorf("删除对象失败: %w", err)
	}

	return nil
}

// ListObjects 列出存储桶中具有给定前缀的对象
func (r *R2Client) ListObjects(ctx context.Context, prefix string, maxKeys int32) ([]string, error) {
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(r.bucketName),
	}

	if prefix != "" {
		input.Prefix = aws.String(prefix)
	}

	if maxKeys > 0 {
		input.MaxKeys = aws.Int32(maxKeys)
	}

	result, err := r.client.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("列出对象失败: %w", err)
	}

	var keys []string
	for _, obj := range result.Contents {
		keys = append(keys, *obj.Key)
	}

	return keys, nil
}

// UploadMultipartFile 上传 multipart 文件到 R2
func (r *R2Client) UploadMultipartFile(ctx context.Context, file *multipart.FileHeader, objectKey string, contentType string) (*UploadResult, error) {
	// 打开上传的文件
	srcFile, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("打开上传文件失败: %w", err)
	}
	defer srcFile.Close()

	// 如果未提供对象键，则使用文件名
	if objectKey == "" {
		// 生成唯一文件名以避免冲突
		fileName := filepath.Base(file.Filename)
		objectKey = fmt.Sprintf("uploads/%s-%s-%s",
			time.Now().Format("20060102"),
			uuid.New().String()[:8],
			fileName)
	}

	// 如果未提供内容类型，尝试从 Header 获取
	if contentType == "" {
		contentType = file.Header.Get("Content-Type")
		// 如果 Header 中没有，根据扩展名推断
		if contentType == "" {
			ext := filepath.Ext(file.Filename)
			contentType = getContentTypeFromExt(ext)
		}
	}

	// 使用 UploadData 上传文件
	return r.UploadData(ctx, srcFile, objectKey, contentType, file.Size)
}

// UploadReader 从任何 io.Reader 上传数据到 R2
// 注意：如果提供的 reader 不是 io.ReadSeeker，此方法会将整个内容读入内存
func (r *R2Client) UploadReader(ctx context.Context, reader io.Reader, objectKey string, contentType string, size int64) (*UploadResult, error) {
	var readSeeker io.ReadSeeker

	// 检查 reader 是否已经是 ReadSeeker
	if rs, ok := reader.(io.ReadSeeker); ok {
		readSeeker = rs
	} else {
		// 如果不是，将所有内容读入内存
		data, err := io.ReadAll(reader)
		if err != nil {
			return nil, fmt.Errorf("读取内容到内存失败: %w", err)
		}
		readSeeker = bytes.NewReader(data)

		// 如果未提供大小，计算大小
		if size <= 0 {
			size = int64(len(data))
		}
	}

	// 使用 UploadData 进行上传
	return r.UploadData(ctx, readSeeker, objectKey, contentType, size)
}

// UploadAny 是一个通用的上传方法，可以处理各种类型的输入
// 支持: 文件路径(string), []byte, io.Reader, *multipart.FileHeader
func (r *R2Client) UploadAny(ctx context.Context, source interface{}, objectKey string, contentType string) (*UploadResult, error) {
	switch src := source.(type) {
	case string:
		// 假设是文件路径
		return r.UploadFile(ctx, src, objectKey, contentType)

	case []byte:
		// 字节数组
		return r.UploadBytes(ctx, src, objectKey, contentType)

	case io.Reader:
		// 任何 io.Reader
		// 注意：在这里无法自动推断大小，除非明确指定或实现 io.Seeker
		return r.UploadReader(ctx, src, objectKey, contentType, -1)

	case *multipart.FileHeader:
		// HTTP 上传的文件
		return r.UploadMultipartFile(ctx, src, objectKey, contentType)

	default:
		return nil, fmt.Errorf("不支持的源类型: %T", source)
	}
}

// getContentTypeFromExt 根据文件扩展名获取内容类型
func getContentTypeFromExt(ext string) string {
	contentType := ""
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg":
		contentType = "image/jpeg"
	case ".png":
		contentType = "image/png"
	case ".gif":
		contentType = "image/gif"
	case ".webp":
		contentType = "image/webp"
	case ".pdf":
		contentType = "application/pdf"
	case ".doc", ".docx":
		contentType = "application/msword"
	case ".xls", ".xlsx":
		contentType = "application/vnd.ms-excel"
	default:
		contentType = defaultContentType
	}
	return contentType
}

// detectContentType 从文件路径检测内容类型的辅助函数
func detectContentType(filePath string) string {
	file, err := os.Open(filePath)
	if err != nil {
		return defaultContentType
	}
	defer file.Close()

	// 读取前 512 字节以检测内容类型
	buffer := make([]byte, 512)
	_, err = file.Read(buffer)
	if err != nil && err != io.EOF {
		return defaultContentType
	}

	// 重置文件位置
	_, err = file.Seek(0, 0)
	if err != nil {
		return defaultContentType
	}

	return http.DetectContentType(buffer)
}
