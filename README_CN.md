# Atlas

一套精练的 Go 后端框架，构建于 Gin 之上，为追求简洁与效率的团队而设计。

Atlas 将认证、存储、缓存、链路追踪等常用基础设施整合为一组内聚的构建模块，通过统一入口接入，开箱即用，零样板代码。

## 快速开始

```go
package main

import (
    "github.com/gin-gonic/gin"
    "github.com/shiliu-ai/go-atlas/atlas"
    "github.com/shiliu-ai/go-atlas/response"
)

func main() {
    a := atlas.New("my-service")

    v1 := a.Group().Group("/v1")
    v1.GET("/health", func(c *gin.Context) {
        response.OK(c, gin.H{"status": "ok"})
    })

    a.MustRun()
}
```

就这些。结构化日志、请求 ID、panic 恢复、CORS、优雅关停——全部自动就绪。

## 安装

```bash
go get github.com/shiliu-ai/go-atlas
```

需要 Go 1.25+。

## 配置

Atlas 底层使用 [Viper](https://github.com/spf13/viper)，在可执行文件同级目录放置 `config.yaml` 即可：

```yaml
server:
  addr: ":8080"
  name: "my-service"
  mode: "release"

log:
  level: "info"          # debug | info | warn | error

auth:
  secret: "change-me"
  issuer: "my-service"
  access_expire: 2h
  refresh_expire: 168h

database:
  driver: "mysql"        # mysql | postgres
  dsn: "user:pass@tcp(127.0.0.1:3306)/mydb?charset=utf8mb4&parseTime=True"
  max_open_conns: 50
  max_idle_conns: 10

redis:
  addr: "127.0.0.1:6379"

storage:
  driver: "s3"           # s3 | cos | oss | tos
  s3:
    endpoint: "https://s3.amazonaws.com"
    region: "us-east-1"
    bucket: "my-bucket"
    access_key_id: ""
    secret_access_key: ""

tracing:
  endpoint: "localhost:4318"
  sample_rate: 1.0

httpclient:
  timeout: 5s
  max_retries: 2

middleware:
  cors:
    allow_origins: ["*"]
  rate_limit:
    rate: 100
    window: 1m
```

所有配置项均为可选。组件按需懒加载——只为你实际使用的部分付出开销。

## 功能模块

### 认证

基于 JWT 的认证体系，支持 access/refresh 令牌对，HS256/384/512 签名算法。

```go
// 生成令牌对
pair, err := a.Auth().GeneratePair(userID, map[string]any{"role": "admin"})

// 保护路由
authorized := v1.Group("/api", auth.Middleware(a.Auth()))

// 提取用户信息
authorized.GET("/me", func(c *gin.Context) {
    claims := auth.ClaimsFromContext(c.Request.Context())
    response.OK(c, gin.H{"user_id": claims.UserID})
})
```

### 数据库

基于 xorm 的 ORM 层，内置连接池管理，支持 MySQL/PostgreSQL，提供上下文感知的事务处理。

```go
db := a.Database()

// 自动回滚的事务
err := database.Transaction(db, func(session *xorm.Session) error {
    _, err := session.Insert(&user)
    return err
})
```

### 缓存

统一的缓存接口，Redis 实现。

```go
cache := a.Cache()
cache.Set(ctx, "key", "value", 5*time.Minute)

val, err := cache.Get(ctx, "key")
```

### 对象存储

一套接口，四家云厂商。切换后端只需改一行配置。

| 驱动 | 提供商 |
|------|--------|
| `s3`  | AWS S3、MinIO 及所有 S3 兼容服务 |
| `cos` | 腾讯云 COS |
| `oss` | 阿里云 OSS |
| `tos` | 火山引擎 TOS |

```go
store := a.Storage()
err := store.Put(ctx, "path/to/file.png", reader, size, "image/png")
url, err := store.SignURL(ctx, "path/to/file.png", 15*time.Minute)
```

### 分布式锁

基于 Redis 的分布式锁，带有持有者令牌和自动过期机制。

```go
l := lock.NewRedisLock(redisClient, "my-lock", 10*time.Second)
acquired, err := l.Acquire(ctx)
defer l.Release(ctx)
```

### HTTP 客户端

生产级 HTTP 客户端，内置重试、指数退避和链路追踪传播。

```go
resp, err := a.HTTPClient().Get(ctx, "https://api.example.com/data")
body := resp.String()

resp, err := a.HTTPClient().PostJSON(ctx, url, payload)
```

### ID 生成

四种策略，适配不同场景：

```go
id.UUID()                       // "550e8400-e29b-41d4-a716-446655440000"
id.NanoID()                     // "V1StGXR8_Z5jdHi6B-myT"
id.ShortID()                    // "0h7a8sK2x9pL3mN1"

sf, _ := id.NewSnowflake(1)
sf.MustGenerate()               // 182439823049723904
```

### 结构化错误

基于错误码的错误体系，与 HTTP 状态码自然映射：

```go
errors.New(errors.CodeNotFound, "用户不存在")
errors.Wrap(err, errors.CodeInternal, "数据库查询失败")

// 在 handler 中使用
response.Fail(c, errors.CodeBadRequest, "邮箱格式不正确")
```

### 请求校验

一步完成绑定与校验，自动返回可读的错误信息：

```go
type CreateUserReq struct {
    Email string `json:"email" binding:"required,email"`
    Name  string `json:"name"  binding:"required,min=2,max=50"`
}

var req CreateUserReq
if !validate.BindJSON(c, &req) {
    return // 错误响应已自动发送
}
```

### 统一响应格式

全 API 一致的 JSON 响应结构：

```go
response.OK(c, data)
// {"code": 0, "message": "ok", "data": {...}, "trace_id": "..."}

response.Fail(c, errors.CodeNotFound, "用户不存在")
// {"code": 404, "message": "用户不存在", "trace_id": "..."}
```

### 分页

```go
authorized.GET("/users", func(c *gin.Context) {
    pg := pagination.FromContext(c)   // 自动绑定 ?page=1&size=20
    users, total := fetchUsers(pg.Offset(), pg.Size)
    response.OK(c, pagination.NewResponse(users, total, pg))
})
```

### 加密

```go
// 密码哈希 (bcrypt)
hash, _ := crypto.HashPassword("secret")
ok := crypto.CheckPassword(hash, "secret")

// AES-GCM 加密
cipher, _ := crypto.NewAES(key)
encrypted, _ := cipher.EncryptString("敏感数据")
decrypted, _ := cipher.DecryptString(encrypted)
```

### OAuth2

```go
provider := oauth.NewProvider("github", oauth.ProviderConfig{
    ClientID:     "...",
    ClientSecret: "...",
    AuthURL:      "https://github.com/login/oauth/authorize",
    TokenURL:     "https://github.com/login/oauth/access_token",
    Scopes:       []string{"user:email"},
})
url := provider.AuthCodeURL("state-token")
```

## 中间件

Atlas 默认注册以下中间件：

| 中间件 | 说明 |
|--------|------|
| **Recovery** | 捕获 panic，记录堆栈，返回 500 |
| **Request ID** | 生成或透传 `X-Request-ID` 请求头 |
| **Tracing** | OpenTelemetry Span 提取，`X-Trace-ID` 响应头 |
| **Logging** | 结构化请求日志，包含耗时、状态码、路径 |
| **CORS** | 可配置的跨域资源共享 |
| **Rate Limit** | 令牌桶（内存）或滑动窗口（Redis） |

日志系统上下文感知——Trace ID 和 Request ID 自动贯穿整条链路。

## 可观测性

Atlas 集成 OpenTelemetry 实现分布式链路追踪：

```yaml
tracing:
  endpoint: "localhost:4318"
  sample_rate: 1.0
  insecure: true
```

链路信息在 HTTP 客户端调用间自动传播，每个响应都携带 `trace_id` 用于端到端排查。

## 架构概览

```
atlas.New()
  ├── Config        (Viper 配置管理)
  ├── Logger        (slog + 彩色终端输出)
  ├── Server        (Gin + 优雅关停)
  ├── Auth          (JWT 认证)
  ├── Database      (xorm ORM)
  ├── Cache         (Redis 缓存)
  ├── Storage       (S3/COS/OSS/TOS 对象存储)
  ├── HTTPClient    (重试 + 链路追踪)
  ├── Tracing       (OpenTelemetry)
  └── Middleware    (恢复, 日志, CORS, 限流, ...)
```

所有组件通过 `sync.Once` 懒加载——首次调用时才创建实例。生命周期管理器负责启动顺序编排，并在收到 SIGINT/SIGTERM 时按注册逆序执行优雅关停。

## 示例

完整可运行示例见 [example/main.go](example/main.go)，涵盖认证、分页、代理转发和 ID 生成。

## 许可证

MIT
