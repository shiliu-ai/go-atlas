# Atlas Architecture Redesign Spec

## 设计理念

**Atlas**（泰坦）掌控全局，**Pillar**（天柱）撑起天穹，**Artifact**（神器）各司其职。

Atlas 是古希腊神话中的泰坦神，承托天穹。框架以此命名，定位为**坚实的基座** — 承载一切应用的基础设施层，同时提供清晰的结构指引。

## 三域架构

整个框架分为三个命名空间，每一层有明确的职责边界：

```
┌─────────────────────────────────────────────────────┐
│                 User Application                    │
├─────────────────────────────────────────────────────┤
│  Pillar（天柱）    Pillar      Pillar      Pillar   │  ← 按需注册
│  database         cache       auth        storage   │
├─────────────────────────────────────────────────────┤
│                  Atlas Core（泰坦）                   │  ← 永远存在
│    Config · Logger · Server · Lifecycle             │
│    Middleware · Response · Errors · I18n             │
├─────────────────────────────────────────────────────┤
│  Artifact（神器）                                     │  ← 独立使用
│  crypto · id · pagination · validate · jsonutil     │
└─────────────────────────────────────────────────────┘
```

| 层 | 命名 | 特征 | 生命周期 |
|----|------|------|----------|
| **Atlas Core** | 泰坦本体 | 永远存在，`New()` 即有 | 框架管理 |
| **Pillar** | 天柱 | 按需注册，有 Init/Stop | 框架管理 |
| **Artifact** | 神器 | 无状态工具库，import 即用 | 无 |

### 分层判定标准

- **Pillar**：需要配置、有连接/资源需要管理、需要优雅关闭 → 放 `pillar/`
- **Artifact**：纯函数或轻量工具、无状态、不依赖配置文件 → 放 `artifact/`
- **Core**：框架运行的必要基础、所有服务都需要 → 放 `atlas/`

特殊情况：`lock`（分布式锁）依赖 Redis 连接，不是无状态工具。它作为 `cache` Pillar 的附属能力提供：

```go
rds := cache.Of(a)
locker := rds.NewLock(key, ttl)
```

## 目录结构

```
go-atlas/
│
├── atlas/                      # Core — 泰坦本体
│   ├── atlas.go                #   Atlas struct · New() · Run()
│   ├── pillar.go               #   Pillar 接口 · Use[T] · TryUse[T]
│   ├── config.go               #   配置加载（内部使用 Viper，不对外暴露）
│   ├── option.go               #   Option 函数类型 · With* 选项
│   ├── core.go                 #   Core struct（Pillar 的有限视图）
│   ├── server.go               #   HTTP server（内部实现，不导出）
│   ├── middleware.go           #   默认中间件链（内部实现）
│   ├── lifecycle.go            #   启动 · 信号监听 · 优雅关闭
│   │
│   ├── errors/                 #   错误体系
│   ├── response/               #   统一响应
│   ├── log/                    #   日志接口
│   └── i18n/                   #   国际化
│
├── pillar/                     # 天柱 — 有生命周期的基础设施组件
│   ├── database/               #   数据库连接管理（GORM）
│   ├── cache/                  #   Redis 缓存（含分布式锁能力）
│   ├── auth/                   #   JWT 认证
│   ├── oauth/                  #   OAuth2 提供商抽象
│   ├── storage/                #   多云对象存储（S3/COS/OSS/TOS）
│   ├── sms/                    #   短信服务
│   ├── tracing/                #   OpenTelemetry 追踪
│   ├── httpclient/             #   生产级 HTTP 客户端
│   └── serviceclient/          #   服务间调用客户端
│
├── artifact/                   # 神器 — 无状态工具库
│   ├── crypto/                 #   密码哈希 · AES-GCM 加密
│   ├── id/                     #   ID 生成（UUID/NanoID/Snowflake）
│   ├── pagination/             #   分页工具
│   ├── validate/               #   请求校验
│   └── jsonutil/               #   JSON 工具
│
└── example/                    # 示例应用
```

### 被吸收的包

以下现有包在重构中被吸收，不再独立存在：

| 现有包 | 去向 | 原因 |
|--------|------|------|
| `config/` | 吸收入 `atlas/config.go` | 配置加载是 Core 内部实现 |
| `server/` | 吸收入 `atlas/server.go` | HTTP server 是 Core 内部实现 |
| `middleware/` | 吸收入 `atlas/middleware.go` | 默认中间件是 Core 内部实现 |
| `app/` | 吸收入 `atlas/lifecycle.go` | 生命周期管理是 Core 内部实现 |
| `lock/` | 吸收入 `pillar/cache/` | 分布式锁依赖 Redis，是 cache 的附属能力 |

### Import 路径

```go
import (
    // Core — 泰坦
    "github.com/example/go-atlas/atlas"
    "github.com/example/go-atlas/atlas/errors"
    "github.com/example/go-atlas/atlas/response"
    "github.com/example/go-atlas/atlas/log"

    // Pillar — 天柱
    "github.com/example/go-atlas/pillar/database"
    "github.com/example/go-atlas/pillar/cache"
    "github.com/example/go-atlas/pillar/auth"

    // Artifact — 神器
    "github.com/example/go-atlas/artifact/crypto"
    "github.com/example/go-atlas/artifact/id"
)
```

## 核心接口设计

### Pillar 接口

```go
// atlas/pillar.go

// Pillar 是所有天柱的统一协议。
type Pillar interface {
    // Name 返回唯一标识，对应配置文件中的 section key。
    Name() string
    // Init 接收 Core 上下文，完成自身初始化。
    Init(core *Core) error
    // Stop 优雅关闭，释放资源。
    Stop(ctx context.Context) error
}

// Starter 由需要后台运行的 Pillar 实现。
type Starter interface {
    Start(ctx context.Context) error
}

// HealthChecker 由支持健康检查的 Pillar 实现。
type HealthChecker interface {
    Health(ctx context.Context) error
}

// MiddlewareProvider 由需要注入全局中间件的 Pillar 实现。
// 返回的中间件插入到默认中间件链之后、用户自定义中间件之前。
type MiddlewareProvider interface {
    Middleware() []gin.HandlerFunc
}
```

设计原则：
- 主接口 `Pillar` 只有三个方法，保持最小化
- `Starter`、`HealthChecker`、`MiddlewareProvider` 为可选扩展接口，按需实现
- 遵循 Go 的小接口哲学 — 接口越小，实现越容易

### Core — Pillar 的有限视图

```go
// atlas/core.go

// Core 是 Atlas 暴露给 Pillar 的有限视图。
// Pillar 通过 Core 访问配置和日志，但看不到其他 Pillar。
type Core struct {
    config *viper.Viper   // 内部字段，不导出
    logger log.Logger
}

// Unmarshal 将指定 key 下的配置反序列化到 target 结构体。
// Pillar 声明自己的 config struct，不接触 Viper。
//
// 用法：
//   var cfg DatabaseConfig
//   core.Unmarshal("databases", &cfg)
func (c *Core) Unmarshal(key string, target any) error

// Logger 返回带有指定名称前缀的子 Logger。
func (c *Core) Logger(name string) log.Logger
```

约束：
- **Pillar 间互不可见。** Core 不暴露其他 Pillar 的引用，强制解耦。如果业务层需要同时使用多个 Pillar，在 main.go 中显式提取并注入。
- **Viper 不泄露。** Pillar 通过 `Unmarshal` 获取配置，只需定义自己的 config struct，与配置后端完全解耦。

### Option 函数类型

`atlas.New()` 的参数同时包含 Pillar 和配置选项。采用函数类型（而非接口）避免未导出方法的包边界问题：

```go
// atlas/option.go

// Option 配置 Atlas 实例。
type Option func(*Atlas)

// New 创建 Atlas 实例。Pillar 和配置选项统一作为 variadic 参数传入。
func New(name string, opts ...Option) *Atlas
```

各 Pillar 包的 `Pillar()` 函数返回 `atlas.Option`：

```go
// pillar/database/pillar.go

// Pillar 返回一个 atlas.Option，将 database Pillar 注册到 Atlas。
func Pillar(opts ...Option) atlas.Option {
    return func(a *atlas.Atlas) {
        m := &Manager{}
        for _, opt := range opts {
            opt(m)
        }
        a.Register(m)
    }
}
```

Atlas 内部的配置选项同理：

```go
// atlas/option.go

func WithMiddleware(mw ...gin.HandlerFunc) Option {
    return func(a *Atlas) { a.extraMiddleware = append(a.extraMiddleware, mw...) }
}

func WithoutDefaultMiddleware() Option {
    return func(a *Atlas) { a.skipDefaultMW = true }
}
```

调用风格自然统一：

```go
a := atlas.New("user-service",
    database.Pillar(),             // Pillar（返回 atlas.Option）
    cache.Pillar(),                // Pillar（返回 atlas.Option）
    atlas.WithMiddleware(mw),      // 配置选项
)
```

### 泛型注册与访问

```go
// atlas/pillar.go

// Atlas 内部通过 map 存储所有已注册的 Pillar。
type Atlas struct {
    // ...core fields...
    pillars map[string]Pillar
}

// Register 将 Pillar 加入注册表。由 Option 函数调用。
func (a *Atlas) Register(p Pillar) {
    if _, exists := a.pillars[p.Name()]; exists {
        panic(fmt.Sprintf("atlas: duplicate pillar name %q", p.Name()))
    }
    a.pillars[p.Name()] = p
}

// Use 以泛型方式取回指定类型的 Pillar，类型安全。
// 如果目标 Pillar 未注册，panic（编程错误，应在开发阶段暴露）。
func Use[T Pillar](a *Atlas) T {
    for _, p := range a.pillars {
        if t, ok := p.(T); ok {
            return t
        }
    }
    var zero T
    panic(fmt.Sprintf("atlas: pillar %T not registered", zero))
}

// TryUse 是 Use 的非 panic 版本，适用于测试和条件逻辑。
func TryUse[T Pillar](a *Atlas) (T, bool) {
    for _, p := range a.pillars {
        if t, ok := p.(T); ok {
            return t, true
        }
    }
    var zero T
    return zero, false
}
```

设计说明：
- `Register` 检测重复 Name，防止静默覆盖
- `Use[T]` panic — 未注册是编程错误，应在开发阶段暴露
- `TryUse[T]` 返回 `(T, bool)` — 测试和条件分支使用，不 panic

### 各 Pillar 包的标准 API

每个 Pillar 包导出两个入口函数：

```go
// pillar/database/pillar.go

// Pillar 返回一个 atlas.Option，将 database Pillar 注册到 Atlas。
func Pillar(opts ...Option) atlas.Option

// Of 从 Atlas 实例中取回已初始化的 database Pillar。
// 读作 "database of atlas"。
func Of(a *atlas.Atlas) *Manager {
    return atlas.Use[*Manager](a)
}
```

命名约定：
- `Pillar()` — 构造函数，返回 `atlas.Option`，用于 `atlas.New()` 的参数
- `Of()` — 访问函数，读作 "database of atlas"

## 生命周期

### 启动流程

```
atlas.New("svc", database.Pillar(), cache.Pillar())
│
├─ 1. Apply Options
│     按顺序执行所有 Option 函数，收集 Pillar 和配置
│
├─ 2. Load Config
│     加载 config.yaml + 环境变量覆盖
│
├─ 3. Init Core
│     创建 Logger、HTTP Server
│
├─ 4. Init Pillars（按注册顺序）
│     ├─ database.Init(core) — Unmarshal databases.* 配置，建立连接池
│     └─ cache.Init(core)    — Unmarshal redis.* 配置，连接 Redis
│     任一 Pillar Init 失败 → 立即 panic，报告哪根柱子没立起来
│
├─ 5. Collect Middleware
│     默认中间件 → Pillar 提供的中间件（按注册顺序）→ 用户自定义中间件
│
└─ 6. Ready
      Atlas 实例可用，用户注册路由

a.Run()
│
├─ 7. Start Pillars
│     调用所有实现了 Starter 接口的 Pillar.Start()
│
├─ 8. Start HTTP Server
│     监听端口，开始服务
│
├─ 9. Wait
│     等待 SIGINT / SIGTERM 信号
│
└─ 10. Graceful Shutdown
       a. Stop HTTP Server（等待在途请求完成）
       b. Stop Pillars（逆注册顺序）
          ├─ cache.Stop()    — 关闭 Redis 连接
          └─ database.Stop() — 关闭数据库连接池
```

关键：HTTP Server 先停（停止接收新请求并等待在途请求完成），然后再关闭 Pillar（确保在途请求还能访问数据库等资源）。

### 失败策略

| 阶段 | 失败行为 | 理由 |
|------|----------|------|
| Config 加载 | panic | 没有配置，无法运行 |
| Pillar Init | panic + 报告哪个 Pillar 失败 | Fail-fast，启动时暴露问题 |
| Pillar Start | 返回 error，Atlas 触发关闭 | 运行时错误，需优雅处理 |
| Pillar Stop | 记录日志，继续关闭其他 Pillar | 关闭阶段不应中断 |

## 配置绑定

### 自动映射规则

每个 Pillar 在 Init 中通过 `core.Unmarshal(name, &cfg)` 读取自己的配置段。`Name()` 返回值对应配置文件中的顶层 key：

```yaml
# config.yaml

# Atlas Core 读取
server:
  port: 8080
  mode: release
log:
  level: info
  format: json

# database Pillar 读取（Name() = "databases"）
databases:
  default:
    driver: mysql
    dsn: "root:password@tcp(127.0.0.1:3306)/mydb"
    max_open_conns: 50

# cache Pillar 读取（Name() = "redis"）
redis:
  addr: "127.0.0.1:6379"
  password: ""
  db: 0

# auth Pillar 读取（Name() = "auth"）
auth:
  secret: "your-jwt-secret"
  access_expire: 2h
  refresh_expire: 168h
```

### 行为规则

- Pillar 已注册 + 配置存在 → 正常初始化
- Pillar 已注册 + 配置缺失 → **由 Pillar 自身决定**是否容忍（通过 Init 返回 error 或 panic）
- Pillar 未注册 + 配置存在 → 忽略（配置文件可以包含未使用的段）

注：配置缺失的处理权交给 Pillar 实现。`core.Unmarshal` 在 key 不存在时返回 error，Pillar 可据此决定是 panic（必需配置）还是使用默认值（可选配置）。这比框架层面统一 panic 更灵活 — 例如 `auth` Pillar 在本地开发时可以接受无配置而使用默认密钥。

### 业务配置

Atlas 在 `Atlas` 上暴露 `Unmarshal` 方法，用户读取自己的业务配置段，方式与 Pillar 读配置完全一致：

```go
// atlas/atlas.go

// Unmarshal 将指定 key 下的配置反序列化到 target。
// 用户在 main.go 中读取业务配置，与 Pillar 读配置的方式统一。
func (a *Atlas) Unmarshal(key string, target any) error
```

使用示例：

```yaml
# config.yaml
server:
  port: 8080
databases:
  default:
    driver: mysql
    dsn: "..."
app:                          # ← 用户自定义的业务配置段
  invite_expire: 72h
  max_team_size: 50
```

```go
// main.go
type AppConfig struct {
    InviteExpire time.Duration `mapstructure:"invite_expire"`
    MaxTeamSize  int           `mapstructure:"max_team_size"`
}

func main() {
    a := atlas.New("user-service",
        database.Pillar(),
    )

    var cfg AppConfig
    a.Unmarshal("app", &cfg)

    svc := service.New(database.Of(a), cfg)
    // ...
}
```

不需要嵌入 `atlas.Config`，不需要实现接口，不需要 `mapstructure:",squash"`。
每个角色只读自己关心的配置段 — Core 读 Core 的，Pillar 读 Pillar 的，用户读用户的。

现有的 `WithCustomConfig` option 不再需要，从 API 中移除。

## 健康检查聚合

Atlas 自动聚合所有实现了 `HealthChecker` 的 Pillar，暴露标准端点：

### 端点

- `GET /healthz` — 完整健康检查（含各 Pillar 状态）
- `GET /livez` — 存活检查（仅 Atlas 进程存活）
- `GET /readyz` — 就绪检查（所有 Pillar 健康才返回 200）

### 响应格式

```json
{
    "status": "healthy",
    "pillars": {
        "database": { "status": "healthy", "latency": "2ms" },
        "cache":    { "status": "healthy", "latency": "1ms" }
    }
}
```

任一 Pillar 不健康 → 整体状态 `unhealthy` + HTTP 503。

无需用户配置，注册了 `HealthChecker` 的 Pillar 自动参与。

## 中间件链

### 执行顺序

中间件分三层，按以下顺序组装：

```
┌─ 第一层：Core 默认中间件（始终存在）─────────────┐
│  1. Recovery         — 捕获 panic，返回 500      │
│  2. Request ID       — 生成/传播 X-Request-ID    │
│  3. I18n             — 从 Accept-Language 检测语言│
│  4. Logging          — 结构化请求日志             │
│  5. CORS             — 跨域访问控制（可配置）      │
│  6. Rate Limit       — 配置中存在时加入           │
├─ 第二层：Pillar 中间件（按注册顺序）────────────── │
│  7. OTel Trace       — tracing Pillar 提供       │
│  8. Forward Headers  — serviceclient Pillar 提供 │
├─ 第三层：用户自定义中间件 ──────────────────────── │
│  9. atlas.WithMiddleware(...) 传入的中间件         │
└──────────────────────────────────────────────────┘
```

Pillar 通过实现 `MiddlewareProvider` 接口注入中间件。Atlas 在 Init 阶段收集，按 Pillar 注册顺序排列，插入到默认中间件之后、用户自定义中间件之前。

用户可通过 option 控制：

```go
atlas.New("svc",
    atlas.WithMiddleware(customMW),        // 追加到第三层
    atlas.WithoutDefaultMiddleware(),      // 关闭第一层所有默认中间件
)
```

## 用户自定义 Pillar

自定义 Pillar 与内置 Pillar 完全平等：

```go
// mypkg/notifier.go

type Config struct {
    WebhookURL string        `mapstructure:"webhook_url"`
    Timeout    time.Duration `mapstructure:"timeout"`
}

type Notifier struct {
    cfg    Config
    client *http.Client
}

func (n *Notifier) Name() string { return "notifier" }

func (n *Notifier) Init(core *atlas.Core) error {
    if err := core.Unmarshal("notifier", &n.cfg); err != nil {
        return fmt.Errorf("notifier: %w", err)
    }
    n.client = &http.Client{Timeout: n.cfg.Timeout}
    return nil
}

func (n *Notifier) Stop(ctx context.Context) error { return nil }

func (n *Notifier) Health(ctx context.Context) error {
    resp, err := n.client.Get(n.cfg.WebhookURL + "/health")
    if err != nil {
        return err
    }
    resp.Body.Close()
    return nil
}

// Pillar 返回 atlas.Option，注册 notifier Pillar。
func Pillar() atlas.Option {
    return func(a *atlas.Atlas) {
        a.Register(&Notifier{})
    }
}

// Of 从 Atlas 中取回 notifier Pillar。
func Of(a *atlas.Atlas) *Notifier {
    return atlas.Use[*Notifier](a)
}
```

```yaml
# config.yaml
notifier:
  webhook_url: "https://hooks.example.com/notify"
  timeout: 5s
```

```go
// main.go
a := atlas.New("svc",
    database.Pillar(),
    notifier.Pillar(),      // 自定义 Pillar，一等公民
)
```

## 完整示例

```go
package main

import (
    "github.com/gin-gonic/gin"

    "github.com/example/go-atlas/atlas"
    "github.com/example/go-atlas/pillar/auth"
    "github.com/example/go-atlas/pillar/cache"
    "github.com/example/go-atlas/pillar/database"
)

func main() {
    // 立柱
    a := atlas.New("user-service",
        database.Pillar(),
        cache.Pillar(),
        auth.Pillar(),
    )

    // 连线 — 所有 Of() 调用集中在此，业务层不碰 Atlas
    authMW := auth.Of(a).Middleware()
    repo := repository.New(database.Of(a))
    svc := service.New(repo, cache.Of(a))
    h := handler.New(svc)

    // 路由
    a.Route(func(r *gin.RouterGroup) {
        r.POST("/login", h.Login)
        r.POST("/refresh", h.Refresh)

        secured := r.Group("/users", authMW)
        {
            secured.POST("/", h.CreateUser)
            secured.GET("/:id", h.GetUser)
            secured.PUT("/:id", h.UpdateUser)
        }
    })

    // 启动
    a.Run()
}
```

## 保留的优势

以下是当前 Atlas 的核心优势，本次重构必须完整保留：

- **极简 API** — `atlas.New()` → `a.Route()` → `a.Run()`，三步启动
- **Convention over Configuration** — 有配置就启用，缺配置由 Pillar 自行决定
- **Fail-Fast** — 启动阶段暴露所有问题，不留运行时隐患
- **统一响应格式** — `R{code, message, data, trace_id}` 全局一致
- **生产级默认值** — 超时、连接池、重试开箱即用

## 解决的问题

| 问题 | 现状 | 重构后 |
|------|------|--------|
| 核心不可扩展 | 每加组件改 Atlas struct + accessor + initComponents | 实现 Pillar 接口即可，零改动 Atlas 核心 |
| 依赖关系隐式 | `a.DB()` 散落在业务代码各处 | main.go 中 `Of()` 提取，显式注入 service 层 |
| 测试困难 | 需要启动完整 Atlas 才能测试业务逻辑 | service 层不依赖 Atlas，直接传 mock |
| 目录结构扁平 | 20+ 包平铺根目录，无层次感 | 三域分层，import 自报家门 |
| 无健康检查 | 缺失 | 自动聚合，开箱即用 |
| 无统一生命周期 | 各组件各自管理 | Pillar 接口统一 Init/Stop |
| 组件无法跳过 | 配置驱动，有配置就初始化 | 显式注册，main.go 即依赖清单 |
| 配置后端泄露 | Pillar 直接依赖 Viper | Core.Unmarshal 隐藏实现，Pillar 只定义 config struct |

## 迁移策略

本 spec 描述的是理想架构蓝图，具体落地节奏在实现计划中确定。大方向：

- 可以分批迁移 — 先建立 Pillar 接口和 Core，再逐个迁移现有组件
- 每迁移一个组件可独立验证，不必一次性全部完成
- Example 应用作为迁移验收标准
- 现有 `config/`、`server/`、`middleware/`、`app/` 包在迁移完成后删除
