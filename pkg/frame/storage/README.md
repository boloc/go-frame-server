# pkg/frame/storage：S3 兼容对象存储客户端

基于 `aws-sdk-go-v2/service/s3`，接入任何实现了 S3 API 的对象存储服务：真正的 AWS S3、
Cloudflare R2、MinIO、Backblaze B2 等。

## 配置项从哪里来

### Cloudflare R2（`storage.R2Config`）

| 字段 | 从哪里拿 |
|---|---|
| `AccountID` | Cloudflare 控制台右侧栏（R2 概览页也会显示），或者直接看你账号下任意一个 R2 存储桶详情页 URL 里 `accounts/<这一段>` |
| `AccessKeyID` / `AccessKeySecret` | R2 → **管理 R2 API 令牌** → 创建 API 令牌，权限选"对象读和写"，创建后立刻复制（Secret 只显示一次，关闭弹窗前务必保存） |
| `BucketName` | R2 → 你要用的存储桶名称，没有就先创建一个 |
| `Region` | R2 不区分真实区域，留空会自动填 `"auto"`，不用管 |
| `PublicBaseURL` | R2 存储桶设置里绑定的自定义域名（推荐），或者接了 CDN 的话填 CDN 域名；留空会退回 R2 的 API 端点拼出来的地址，那个地址**默认不能公开访问**，除非你在桶设置里手动开了"公共访问" |

### 真正的 AWS S3 / MinIO 等其它 S3 兼容服务（`storage.S3Config`）

| 字段 | 从哪里拿 |
|---|---|
| `AccessKeyID` / `AccessKeySecret` | AWS：IAM → 用户 → 安全凭证 → 创建访问密钥（建议只授予目标桶的读写权限，不要用根账号密钥）。MinIO：控制台或 `mc admin user` 创建的密钥 |
| `BucketName` | 目标桶名 |
| `Region` | AWS：桶所在的真实区域（如 `us-east-1`、`ap-southeast-1`），在桶详情页能看到。MinIO/其它兼容服务：随便填一个非空值即可，服务端通常不校验 |
| `Endpoint` | AWS 真实 S3：**留空**，SDK 会按 `Region` 自动算出官方端点，不要自己拼。MinIO/其它兼容服务：必须填对方文档给的端点地址（如自建 MinIO 的 `https://minio.example.com`） |
| `UsePathStyle` | AWS 真实 S3：`false`（默认，虚拟主机风格）。MinIO/大多数 S3 兼容服务：`true`（路径风格），具体看对方文档，配错了上传会报 404/签名错误 |
| `PublicBaseURL` | 自定义域名或 CDN 域名；留空同样会退回 `Endpoint` 拼出来的地址，公开访问性取决于对方服务和桶的权限配置 |

## 推荐用法：从配置文件读取，不要把密钥硬编码在代码里

跟框架里 MySQL/Redis 的 `Setup*` 一样的写法，在 `config/frame-server.yml` 里加一段：

```yaml
storage:
  r2:
    account_id: ""
    access_key_id: ""
    access_key_secret: ""
    bucket_name: ""
    public_base_url: "" # 建议配置自定义域名，否则退回 R2 的 API 端点（默认不公开可访问）
```

在 bootstrap 里读出来创建客户端。这个包本身没有 Start/Stop 生命周期，不需要注册成
`frame.Component`；对象存储是可选能力（不是每个应用都需要），构造好之后用
`storage.SetDefault` 存成进程内默认实例，业务代码用 `storage.TryDefault()` 取，取不到
就说明没配置，自己决定要不要报错——完整实现见 `cmd/example/bootstrap/storage.go`：

```go
func SetupStorage(conf *config.ConfigComponent) {
	accountID := conf.GetString("storage.r2.account_id")
	if accountID == "" {
		logger.Info("storage: 未配置 storage.r2.account_id，跳过对象存储初始化")
		return
	}

	client, err := storage.NewR2Client(context.Background(), storage.R2Config{
		AccountID:       accountID,
		AccessKeyID:     conf.GetString("storage.r2.access_key_id"),
		AccessKeySecret: conf.GetString("storage.r2.access_key_secret"),
		BucketName:      conf.GetString("storage.r2.bucket_name"),
		PublicBaseURL:   conf.GetString("storage.r2.public_base_url"),
	})
	if err != nil {
		panic("bootstrap: 初始化对象存储失败: " + err.Error())
	}
	storage.SetDefault(client)
}
```

```go
// 业务代码里取用：
client, ok := storage.TryDefault()
if !ok {
	// 没配置对象存储，按业务需要决定是报错还是跳过
}
```

## 接入示例（直接用 Go 结构体字面量，适合脚本/单测场景）

```go
import "github.com/boloc/go-frame-server/pkg/frame/storage"

client, err := storage.NewR2Client(ctx, storage.R2Config{
    AccountID:       "your-cloudflare-account-id",
    AccessKeyID:     "your-r2-access-key-id",
    AccessKeySecret: "your-r2-access-key-secret",
    BucketName:      "your-bucket-name",
    PublicBaseURL:   "https://cdn.example.com",
})
```

```go
// 真正的 AWS S3 / MinIO 等
client, err := storage.NewS3Client(ctx, storage.S3Config{
    AccessKeyID:     "...",
    AccessKeySecret: "...",
    BucketName:      "...",
    Region:          "us-east-1",
})
```

## 可以直接跑的示例接口

`cmd/example` 挂了一组演示路由（`/test/storage/*`，需要 `X-Demo-Token` 请求头），配置好
`storage.r2` 之后可以直接 curl 验证，见 `cmd/example/route/storage_route.go` 顶部注释里
的完整命令。没配置时这几个接口会返回"对象存储未配置"的业务错误，不会 panic。

## 上传

```go
result, err := client.UploadBytes(ctx, data, "hello.txt", "text/plain")
// 或 UploadFile / UploadReader / UploadMultipartFile / UploadAny
fmt.Println(result.URL, result.Key, result.Size)
```

## 其它能力

- `GeneratePresignedURL(ctx, key, expires)`：限时访问的预签名 URL。
- `ListObjects(ctx, prefix, maxKeys)`：按前缀列出对象键。
- `DeleteObject(ctx, key)`：删除对象。

## 最佳实践

1. 复用同一个 `*S3Client` 实例（内部持有连接池），不要每次请求都 `NewS3Client`。
2. 密钥从配置文件/环境变量读取，不要硬编码进代码或提交进 git。
3. 用唯一的对象键，避免覆盖已有文件。
4. 用户上传的内容，上传前校验文件类型和大小。
5. 私有对象用预签名 URL 做临时访问，不要放公开 URL。
6. 上传大文件时给 ctx 设置合适的超时。
