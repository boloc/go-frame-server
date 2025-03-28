# Cloudflare R2 文件上传工具类

本工具类提供了一个优雅的接口，用于上传文件到 Cloudflare R2 存储服务。它使用 AWS SDK for Go v2，因为 Cloudflare R2 与 S3 API 兼容。

## 功能特点

- 从磁盘上传文件
- 直接上传字节数组
- 生成临时访问的预签名 URL
- 按前缀筛选列出存储桶中的对象
- 删除对象
- 内容类型自动检测

## 安装

要使用此工具类，请确保已安装所需的依赖项：

```sh
go get github.com/aws/aws-sdk-go-v2/aws
go get github.com/aws/aws-sdk-go-v2/config
go get github.com/aws/aws-sdk-go-v2/credentials
go get github.com/aws/aws-sdk-go-v2/service/s3
go get github.com/google/uuid
```

## 配置

在使用该工具类之前，您需要设置 Cloudflare R2 凭据：

1. 在 Cloudflare 控制面板中创建一个 R2 存储桶
2. 生成具有适当权限的访问密钥 ID 和秘密访问密钥
3. 记下您的账户 ID（在控制面板 URL 中可见）

## 使用方法

### 创建客户端

```go
import (
    "nav-market/pkg/util"
    "context"
)

r2Config := &util.R2Config{
    AccountID:       "your-cloudflare-account-id",
    AccessKeyID:     "your-r2-access-key-id",
    AccessKeySecret: "your-r2-access-key-secret",
    BucketName:      "your-bucket-name",
    Region:          "auto", // 通常对于 R2 使用 "auto"
}

r2Client, err := util.NewR2Client(r2Config)
if err != nil {
    // 处理错误
}
```

### 从磁盘上传文件

```go
ctx := context.Background()
result, err := r2Client.UploadFile(ctx, "/path/to/file.jpg", "uploads/file.jpg", "image/jpeg")
if err != nil {
    // 处理错误
}

// 访问已上传文件的详细信息
fmt.Println("文件 URL:", result.URL)
fmt.Println("文件键名:", result.Key)
fmt.Println("文件大小:", result.Size)
fmt.Println("内容类型:", result.ContentType)
```

如果未提供内容类型，系统将自动检测。

### 上传字节数据

```go
data := []byte("你好，世界！")
result, err := r2Client.UploadBytes(ctx, data, "hello.txt", "text/plain")
if err != nil {
    // 处理错误
}
```

### 生成预签名 URL

```go
// 生成一个有效期为 1 小时的 URL
url, err := r2Client.GeneratePresignedURL(ctx, "uploads/file.jpg", 1 * time.Hour)
if err != nil {
    // 处理错误
}
fmt.Println("预签名 URL:", url)
```

### 列出对象

```go
// 列出前缀为 "uploads/" 的最多 10 个对象
objects, err := r2Client.ListObjects(ctx, "uploads/", 10)
if err != nil {
    // 处理错误
}

for _, key := range objects {
    fmt.Println(key)
}
```

### 删除对象

```go
err := r2Client.DeleteObject(ctx, "uploads/file.jpg")
if err != nil {
    // 处理错误
}
```

## 完整示例

请参阅 `examples/r2upload_example.go` 获取完整的使用示例。

## 错误处理

所有方法都返回有意义的错误消息，包括适用的底层 AWS SDK 错误。这使得更容易排查 R2 存储操作中的问题。

## 最佳实践

1. 重用 R2Client 实例以利用连接池
2. 使用唯一的对象键以避免覆盖现有文件
3. 对于用户上传的内容，考虑在上传前验证文件类型和大小
4. 使用预签名 URL 允许对私有对象进行安全的临时访问
5. 上传大文件时设置适当的上下文超时值

## 支持

有关此工具类的问题或疑问，请联系开发团队。 
