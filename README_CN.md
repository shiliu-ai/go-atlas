# Atlas

[English](README.md)

一套精练的 Go 后端框架，构建于 Gin 之上，为追求简洁与效率的团队而设计。

Atlas 将认证、存储、缓存、链路追踪、服务间通信等常用基础设施整合为一组内聚的构建模块，采用**四域架构**，开箱即用，零样板代码。

## 快速开始

```go
package main

import (
    "github.com/gin-gonic/gin"
    atlas "github.com/shiliu-ai/go-atlas"
    "github.com/shiliu-ai/go-atlas/aether/response"
)

func main() {
    a := atlas.New("my-service")

    a.Route(func(r *gin.RouterGroup) {
        r.GET("/health", func(c *gin.Context) {
            response.OK(c, gin.H{"status": "ok"})
        })
    })

    a.MustRun()
}
```

就这些。结构化日志、请求 ID、panic 恢复、CORS、i18n、优雅关停——全部自动就绪。

## 安装

```bash
go get github.com/shiliu-ai/go-atlas
```

需要 Go 1.25+。

## 架构概览

Atlas 按四个领域组织代码，命名均源自希腊神话：

| 领域 | 名称 | 含义 | 职责 |
|------|------|------|------|
| 核心 | **Atlas** | 阿特拉斯 | 框架编排器 |
| 内置 | **Aether** | 以太 | 必备基础组件 |
| 扩展 | **Pillar** | 立柱 | 可选生命周期组件 |
| 工具 | **Artifact** | 神器 | 独立工具集 |

```
*.go                核心（package atlas）— 配置、服务器、生命周期、中间件、Pillar 接口
aether/             以太 — 内置必备组件
  ├── errors        结构化错误码
  ├── i18n          国际化
  ├── log           结构化日志
  └── response      统一 API 响应
pillar/             立柱 — 通过 Pillar() 注册的可插拔组件
  ├── auth          JWT 认证
  ├── cache         Redis 缓存 + 分布式锁
  ├── database      GORM（MySQL/PostgreSQL，多命名连接）
  ├── httpclient    生产级 HTTP 客户端
  ├── oauth         OAuth2 提供商
  ├── serviceclient 类型化服务间 RPC
  ├── sms           短信发送（腾讯云）
  ├── storage       对象存储（S3/COS/OSS/TOS）
  └── tracing       OpenTelemetry 分布式链路追踪
artifact/           神器 — 独立辅助工具（加密、ID 生成、分页、校验）
```

### Pillar 模式

每个基础设施组件遵循相同的模式：

```go
// 1. 立柱：在 atlas.New() 中注册 Pillar
a := atlas.New("my-service",
    auth.Pillar(),
    database.Pillar(),
    cache.Pillar(),
)

// 2. 连线：通过 Of() 获取已初始化的实例
jwt := auth.Of(a)
dbm := database.Of(a)
redis := cache.Of(a)
```

所有 Pillar 实现 `atlas.Pillar` 接口：

```go
type Pillar interface {
    Name() string
    Init(core *Core) error
    Stop(ctx context.Context) error
}
```

Pillar 可选实现 `Starter`（后台协程）、`HealthChecker`（健康检查）或 `MiddlewareProvider`（自动注入中间件）。

## 配置

Atlas 底层使用 [Viper](https://github.com/spf13/viper)，在可执行文件同级目录放置 `config.yaml` 即可：

```yaml
server:
  port: 8080
  name: "my-service"
  mode: "release"           # debug | release | test
  read_timeout: 30s
  write_timeout: 30s
  shutdown_timeout: 10s

log:
  level: "info"             # debug | info | warn | error
  format: "text"            # text (默认) | json

i18n:
  default: "en"             # 默认语言标签，如 "en"、"zh-Hans"

auth:
  secret: "change-me"
  issuer: "my-service"
  access_expire: 2h
  refresh_expire: 168h

databases:
  default:
    driver: "mysql"         # mysql | postgres
    dsn: "user:pass@tcp(127.0.0.1:3306)/mydb?charset=utf8mb4&parseTime=True"
    max_open_conns: 50
    max_idle_conns: 10
    max_lifetime: 1h
    log_level: "info"
  # readonly:
  #   driver: "mysql"
  #   dsn: "user:pass@tcp(127.0.0.1:3307)/mydb?charset=utf8mb4&parseTime=True"

redis:
  addr: "127.0.0.1:6379"

storages:
  default:
    driver: "s3"            # s3 | cos | oss | tos
    s3:
      endpoint: "https://s3.amazonaws.com"
      region: "us-east-1"
      bucket: "my-bucket"
      access_key_id: ""
      secret_access_key: ""
  # backup:
  #   driver: "cos"
  #   cos:
  #     bucket_url: "https://<bucket>-<appid>.cos.<region>.myqcloud.com"

# 短信（腾讯云）
sms:
  default:
    driver: "tencentcloud"
    tencent:
      secret_id: "${SMS_SECRET_ID}"
      secret_key: "${SMS_SECRET_KEY}"
      app_id: "1400000000"
      sign: "YourAppName"
      region: "ap-guangzhou"

tracing:
  service_name: "my-service"
  endpoint: "localhost:4318"
  sample_rate: 1.0
  insecure: true

httpclient:
  timeout: 5s
  max_retries: 2
  retry_wait: 500ms

services:
  user-service:
    base_url: "http://user-service:8080/user-service"
    timeout: 5s             # 覆盖全局 httpclient 超时
    max_retries: 3          # 覆盖全局 httpclient 重试次数

middleware:
  cors:
    allow_origins: ["*"]
    allow_methods: ["GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"]
    allow_headers: ["Origin", "Content-Type", "Authorization", "X-Request-ID"]
    max_age: 86400
  rate_limit:
    rate: 100
    window: 1m
```

所有配置项均为可选。各 Pillar 在 `Init()` 阶段读取自身配置段——配置错误会触发 panic 快速失败。

环境变量同样支持，默认前缀为 `APP_`（可通过 `WithEnvPrefix` 自定义）。嵌套键使用下划线分隔：`APP_SERVER_PORT=9090`。

## 功能模块

### 认证

基于 JWT 的认证体系，支持 access/refresh 令牌对，HS256/384/512 签名算法。

```go
a := atlas.New("my-service", auth.Pillar())

jwt := auth.Of(a)

// 生成令牌对
pair, err := jwt.GeneratePair(userID, map[string]any{"role": "admin"})

// 保护路由
a.Route(func(r *gin.RouterGroup) {
    authorized := r.Group("/api", jwt.Middleware())

    // 提取用户信息
    authorized.GET("/me", func(c *gin.Context) {
        claims := auth.ClaimsFromContext(c.Request.Context())
        response.OK(c, gin.H{"user_id": claims.UserID})
    })
})
```

### 数据库

基于 GORM 的 ORM 层，支持多个命名连接、懒初始化、连接池管理，兼容 MySQL/PostgreSQL。

```go
a := atlas.New("my-service", database.Pillar())

dbm := database.Of(a)

// 默认连接
db, err := dbm.Default()

// 命名连接（如只读副本）
db, err := dbm.Get("readonly")
```

### 缓存

Redis 客户端，统一缓存接口，内置分布式锁。

```go
a := atlas.New("my-service", cache.Pillar())

redis := cache.Of(a)
redis.Set(ctx, "key", "value", 5*time.Minute)
val, err := redis.Get(ctx, "key")

// 分布式锁
l := redis.NewLock("my-lock", 10*time.Second)
acquired, err := l.Acquire(ctx)
defer l.Release(ctx)
```

### 对象存储

一套接口，四家云厂商。切换后端只需改一行配置。支持多个命名存储实例。

| 驱动 | 提供商 |
|------|--------|
| `s3`  | AWS S3、MinIO 及所有 S3 兼容服务 |
| `cos` | 腾讯云 COS |
| `oss` | 阿里云 OSS |
| `tos` | 火山引擎 TOS |

```go
a := atlas.New("my-service", storage.Pillar())

stm := storage.Of(a)

// 默认存储
store, err := stm.Get("default")
err = store.Put(ctx, "path/to/file.png", reader, size, "image/png")
url, err := store.SignURL(ctx, "path/to/file.png", 15*time.Minute)

// 命名存储
store, err := stm.Get("backup")
```

### 短信（SMS）

统一短信发送接口，支持多服务商（腾讯云）；命名实例支持多租户/OEM 场景。

```go
a := atlas.New("my-service", sms.Pillar())

smsMgr := sms.Of(a)
s, err := smsMgr.Default()
err = s.Send(ctx, &sms.SendRequest{
    Phone:      "+8613800138000",
    TemplateID: "123456",
    Params:     []string{"1234", "5"},
})
```

### 服务间通信

为调用其他 Atlas 服务提供的类型化 HTTP 客户端。自动解包标准 `R{code, message, data}` 响应信封，转发请求头（Authorization、X-Request-ID、X-Trace-ID），支持按服务配置超时/重试。

```go
a := atlas.New("my-service",
    httpclient.Pillar(),
    serviceclient.Pillar(),
)

svcm := serviceclient.Of(a)
userSvc := svcm.MustGet("user-service")

// 类型化调用——response.data 自动反序列化到目标结构
var user User
err := serviceclient.Get(ctx, userSvc, "/v1/users/123", &user)

// 带查询参数
var users []User
err := serviceclient.Get(ctx, userSvc, "/v1/users", &users,
    serviceclient.WithQuery(url.Values{"page": {"1"}, "size": {"20"}}),
)

// POST 带请求体
var created User
err := serviceclient.Post(ctx, userSvc, "/v1/users", createReq, &created)
```

### HTTP 客户端

生产级 HTTP 客户端，内置重试、指数退避和链路追踪传播。

```go
a := atlas.New("my-service", httpclient.Pillar())

hc := httpclient.Of(a)
resp, err := hc.Get(ctx, "https://api.example.com/data")
body := resp.String()

resp, err := hc.PostJSON(ctx, url, payload)
```

### ID 生成

四种策略，适配不同场景（独立工具，无需 Pillar）：

```go
id.UUID()                       // "550e8400-e29b-41d4-a716-446655440000"
id.NanoID()                     // "V1StGXR8_Z5jdHi6B-myT"
id.ShortID()                    // "0h7a8sK2x9pL3mN1"

sf, _ := id.NewSnowflake(1)
sf.MustGenerate()               // 182439823049723904
```

### 结构化错误

基于错误码的错误体系，与 HTTP 状态码自然映射。支持 i18n 消息键。

```go
errors.New(errors.CodeNotFound, "用户不存在")
errors.NewT(errors.CodeNotFound, "error.user_not_found")  // i18n 键
errors.Wrap(errors.CodeInternal, "数据库查询失败", err)

// 预定义哨兵错误
errors.ErrNotFound          // 404
errors.ErrUnauthorized      // 401
errors.ErrBadRequest        // 400

// 流式 API
errors.ErrNotFound.WithMessage("用户不存在")
errors.ErrNotFound.WithMsgKey("error.user_not_found")

// 在 handler 中使用
response.Fail(c, errors.CodeBadRequest, "邮箱格式不正确")
response.FailT(c, errors.CodeBadRequest, "error.invalid_email")  // i18n
response.Err(c, err)  // 自动识别 *errors.Error
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

response.Err(c, err)        // 从 *errors.Error 推导响应
response.AbortErr(c, err)   // 同 Err 但中止中间件链
```

### 分页

```go
authorized.GET("/users", func(c *gin.Context) {
    pg := pagination.FromContext(c)   // 自动绑定 ?page=1&size=20
    users, total := fetchUsers(pg.Offset(), pg.Size)
    response.OK(c, pagination.NewResponse(users, total, pg))
})
```

### 国际化 (i18n)

内置 i18n 支持，通过 `Accept-Language` 请求头自动检测语言环境。

```go
// 注册自定义翻译
bundle := a.I18nBundle()
bundle.Register(language.English, map[string]string{
    "error.user_not_found": "User not found",
})
bundle.Register(language.SimplifiedChinese, map[string]string{
    "error.user_not_found": "用户不存在",
})

// 在响应中使用 i18n
response.FailT(c, errors.CodeNotFound, "error.user_not_found")
```

### 加密

独立工具，无需 Pillar：

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
a := atlas.New("my-service", oauth.Pillar())

oauthMgr := oauth.Of(a)
github := oauthMgr.MustGet("github")
url := github.AuthCodeURL("state-token")
```

### 自定义配置

使用 `a.Unmarshal()` 从同一配置文件读取自定义配置段：

```go
type BusinessConfig struct {
    MaxItems int    `mapstructure:"max_items"`
    Region   string `mapstructure:"region"`
}

var bizCfg BusinessConfig
if err := a.Unmarshal("business", &bizCfg); err != nil {
    panic(err)
}
```

## 中间件

Atlas 默认注册以下中间件：

| 中间件 | 说明 |
|--------|------|
| **Recovery** | 捕获 panic，记录堆栈，返回 500 |
| **Request ID** | 生成或透传 `X-Request-ID` 请求头 |
| **I18n** | 从 `Accept-Language` 请求头检测语言环境 |
| **Logging** | 结构化请求日志，包含耗时、状态码、路径 |
| **CORS** | 可配置的跨域资源共享 |
| **Rate Limit** | 滑动窗口限流器（需配置） |

实现 `MiddlewareProvider` 接口的 Pillar（如 tracing、serviceclient）会自动注入中间件，位于核心默认中间件与用户自定义中间件之间。

日志系统上下文感知——Trace ID 和 Request ID 自动贯穿整条链路。

使用 `WithoutDefaultMiddleware()` 可禁用所有默认中间件，使用 `WithMiddleware(...)` 可添加自定义中间件。

## 可观测性

注册 tracing Pillar 即可启用 OpenTelemetry 分布式链路追踪：

```go
a := atlas.New("my-service", tracing.Pillar())
```

```yaml
tracing:
  service_name: "my-service"
  endpoint: "localhost:4318"
  sample_rate: 1.0
  insecure: true
```

链路信息在 HTTP 客户端调用和服务间通信中自动传播，每个响应都携带 `trace_id` 用于端到端排查。

## 示例

完整可运行示例见 [example/main.go](example/main.go)，涵盖认证、分页、服务间调用、代理转发和 ID 生成。

## 许可证

MIT
