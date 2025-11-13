# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

Wakapi 是一个开源的、自托管的 WakaTime 兼容后端服务，用于追踪和统计编程时间。项目使用 Go 语言编写，采用分层架构模式，支持多种数据库（SQLite、MySQL、PostgreSQL）。

## 开发环境要求

- **Go**: 1.25+ (使用 `go.mod` 管理依赖)
- **Node.js**: 用于前端资源构建
- **包管理器**: npm 或 yarn (项目使用 npm)
- **数据库**: 默认使用 SQLite，生产环境推荐 MySQL/PostgreSQL

## 常用命令

### 构建和运行

```bash
# 编译项目
go build -o wakapi main.go

# 开发模式运行（使用默认配置）
./wakapi -config config.yml

# 快速运行（跳过初始化任务，仅用于开发）
WAKAPI_QUICK_START=true ./wakapi

# 查看版本
./wakapi -version
```

### 前端资源构建

```bash
# 安装依赖
npm install

# 构建所有前端资源（TailwindCSS + Icons）
npm run build

# 监听文件变化并自动构建
npm run watch

# 构建并压缩（Brotli）
npm run build:all:compress

# 仅构建 TailwindCSS
npm run build:tailwind

# 仅构建图标
npm run build:icons
```

### 测试

```bash
# 运行所有单元测试（推荐安装 tparse 用于美化输出）
go install github.com/mfridman/tparse@latest
CGO_ENABLED=0 go test -json -coverprofile=coverage/coverage.out ./... -run ./... | tparse -all

# 运行单元测试（不带美化输出）
CGO_ENABLED=0 go test -v ./...

# 运行特定包的测试
CGO_ENABLED=0 go test -v ./services

# 运行特定测试函数
CGO_ENABLED=0 go test -v -run TestDurationService ./services

# API 测试（需要 Bruno CLI）
npm install -g @usebruno/cli
./testing/run_api_tests.sh           # SQLite
./testing/run_api_tests.sh postgres  # PostgreSQL
./testing/run_api_tests.sh mysql     # MySQL

# 邮件测试
./testing/run_mail_tests.sh
```

### API 文档生成

```bash
# 安装 swag 工具
go install github.com/swaggo/swag/cmd/swag@latest

# 生成 Swagger 文档
swag init -o static/docs

# 查看文档：启动服务后访问
# http://localhost:3000/swagger-ui
```

### 数据库操作

```bash
# 数据库迁移会自动运行，除非设置：
WAKAPI_SKIP_MIGRATIONS=true ./wakapi

# 如果需要手动运行迁移，启动应用即可（默认行为）
./wakapi -config config.yml
```

## 架构设计

### 分层架构（Clean Architecture）

项目采用经典的三层架构模式，遵循依赖倒置原则：

```
┌──────────────────────────┐
│   Routes (Controllers)   │  ← HTTP 请求处理、路由
├──────────────────────────┤
│   Services (Business)    │  ← 业务逻辑、用例
├──────────────────────────┤
│   Repositories (Data)    │  ← 数据访问、持久化
└──────────────────────────┘
         ↓
    ┌─────────┐
    │ Models  │  ← 数据模型、实体
    └─────────┘
```

**注意**: `main.go:93` 有一个 TODO 注释表明未来可能重构为业务域驱动架构（DDD）。

### 核心目录说明

- **`models/`**: 数据模型和实体定义
  - `user.go`: 用户模型（包含 30+ 字段，如 API Key、邮箱、心跳超时配置等）
  - `heartbeat.go`: 核心心跳数据模型（包含去重哈希、文件路径、项目、语言等）
  - `summary.go`: 统计摘要模型（聚合数据）
  - `compat/`: WakaTime 兼容模型
  - `metrics/`: Prometheus 指标模型

- **`repositories/`**: 数据访问层（15 个文件）
  - 每个 repository 对应一个数据模型
  - 使用 GORM 作为 ORM
  - 基类：`base.go` 提供通用的数据库操作

- **`services/`**: 业务逻辑层（24 个文件）
  - 13 个核心服务接口（定义在 `services/services.go`）
  - `heartbeat.go`: 心跳数据处理（核心功能）
  - `summary.go`: 统计摘要生成
  - `aggregation.go`: 数据聚合服务（定时任务）
  - `mail/`: 邮件发送服务
  - `imports/`: 数据导入服务（WakaTime/Wakapi）

- **`routes/`**: 路由和控制器（21 个文件）
  - `api/`: REST API 路由
  - `compat/wakatime/v1/`: WakaTime v1 兼容 API
  - `compat/shields/v1/`: Shields.io 徽章 API
  - `relay/`: 心跳转发服务

- **`middlewares/`**: 中间件（11 个文件）
  - `authenticate.go`: 认证中间件（支持 Cookie、API Key、Trusted Header、OIDC）
  - `logging.go`: 请求日志
  - `security.go`: 安全相关（CORS、Rate Limiting）

- **`config/`**: 配置管理（17 个文件）
  - `config.go`: 核心配置逻辑
  - `db.go`: 数据库配置
  - `oidc.go`: OpenID Connect 配置
  - 支持 YAML 文件和环境变量两种配置方式

- **`migrations/`**: 数据库迁移（34 个迁移文件）
  - 从 2020 年到 2025 年的迁移历史
  - 自动执行，除非设置 `skip_migrations: true`

- **`helpers/`**: 辅助函数（8 个文件）
- **`utils/`**: 工具函数（21 个文件）
- **`views/`**: HTML 模板（25 个文件，使用 Go Template）
- **`static/`**: 静态资源（CSS、JS、图片）

### 依赖注入流程

在 `main.go` 中按顺序初始化（手动依赖注入）：

```go
1. 初始化数据库连接 (GORM)
2. 运行数据库迁移
3. 初始化 Repositories (注入 db)
4. 初始化 Services (注入 repositories)
5. 初始化 Handlers/Routes (注入 services)
6. 配置路由和中间件
7. 启动 HTTP 服务器
```

## 心跳数据处理机制

这是 Wakapi 的核心功能，需要理解其工作原理：

### 心跳超时配置

```go
// models/user.go
HeartbeatsTimeoutSec int  // 用户可配置，默认 10 分钟

// 时间范围限制
DefaultHeartbeatsTimeout = 10 * time.Minute
MinHeartbeatsTimeout = 1 * time.Minute
MaxHeartbeatsTimeout = 1 * time.Hour
```

### 时长计算逻辑

心跳之间的时间间隔超过配置的超时时间时，会被视为"中断"：

```text
示例（超时设置为 10 分钟）：
|---o---o--------------o---o---|
|   |10s|      3m      |10s|   |

计算时长：10s + 3m + 10s = 3分20秒
（因为 3 分钟 < 10 分钟超时）
```

详见 `services/duration.go` 和 `README.md` 的 FAQ 部分。

### 心跳去重

```go
// models/heartbeat.go
Hash string `gorm:"type:varchar(17); uniqueIndex"`
```

使用哈希值确保同一时间的相同心跳不会重复记录。

## 定时任务

项目使用 `robfig/cron` 库实现定时任务：

```yaml
# config.default.yml
app:
  aggregation_time: '0 15 2 * * *'                          # 每天 02:15 - 摘要聚合
  leaderboard_generation_time: '0 0 6 * * *,0 0 18 * * *'   # 每天 06:00 和 18:00 - 排行榜
  report_time_weekly: '0 0 18 * * 5'                        # 每周五 18:00 - 周报
  data_cleanup_time: '0 0 6 * * 0'                          # 每周日 06:00 - 数据清理
  optimize_database_time: '0 0 8 1 * *'                     # 每月 1 号 08:00 - 数据库优化
```

## 认证机制

Wakapi 支持 4 种认证方式（`middlewares/authenticate.go`）：

1. **Cookie 认证**: 浏览器会话（`wakapi_auth` Cookie）
2. **API Key (Header)**: `Authorization: Basic <BASE64_API_KEY>`
3. **API Key (Query)**: `?api_key=xxx`
4. **Trusted Header**: 反向代理 SSO（需谨慎配置）
5. **OpenID Connect**: 外部身份提供商（支持多个 OIDC Provider）

## 配置管理

### 配置优先级

1. 环境变量（前缀 `WAKAPI_`，优先级最高）
2. Docker Secrets（支持 `_FILE` 后缀）
3. YAML 配置文件（默认 `config.yml`）

### 关键配置项

```yaml
# 必须设置的配置
security:
  password_salt: ''  # ⚠️ 必须设置！用于密码哈希的 pepper

# 开发环境配置
env: dev  # 或 production
security:
  insecure_cookies: true  # 生产环境必须设置为 false 并启用 HTTPS

# 数据库配置
db:
  dialect: sqlite3  # 或 mysql, postgres
  name: wakapi_db.db
  max_conn: 2  # 连接池大小
```

## 数据库支持

### 支持的数据库

- **SQLite** (默认) - 适合单用户/小型部署
- **MySQL** (推荐生产环境) - 最广泛测试
- **MariaDB** - MySQL 开源替代
- **PostgreSQL** - 开源高性能

### 连接配置

```yaml
# SQLite
db:
  dialect: sqlite3
  name: wakapi_db.db

# MySQL/MariaDB
db:
  dialect: mysql
  host: localhost
  port: 3306
  user: wakapi
  password: secret
  name: wakapi
  charset: utf8mb4

# PostgreSQL
db:
  dialect: postgres
  host: localhost
  port: 5432
  user: wakapi
  password: secret
  name: wakapi
  ssl: false
```

## API 开发

### Swagger 注解

API 使用 `swaggo/swag` 生成文档，在函数上方添加注解：

```go
// @Summary Get user's summary
// @Tags summary
// @Accept json
// @Produce json
// @Param user path string true "User ID"
// @Success 200 {object} models.Summary
// @Failure 401 {string} string "unauthorized"
// @Security ApiKeyAuth
// @Router /summary/{user} [get]
func (h *SummaryHandler) Get(w http.ResponseWriter, r *http.Request) {
    // ...
}
```

### REST API 路由结构

```
/api
├── /health              # 健康检查
├── /users               # 用户管理
├── /heartbeats          # 心跳数据
├── /summary             # 统计摘要
├── /metrics             # Prometheus 指标（需启用）
└── /compat
    ├── /wakatime/v1     # WakaTime 兼容 API
    └── /shields/v1      # Shields.io 徽章
```

## WakaTime 兼容性

Wakapi 部分兼容 WakaTime API：

- 兼容 WakaTime 客户端（`~/.wakatime.cfg` 配置）
- 支持心跳转发到 WakaTime
- 支持从 WakaTime 导入历史数据
- 兼容 API 端点：`/api/compat/wakatime/v1/`

## 常见开发任务

### 添加新的数据模型

1. 在 `models/` 中定义模型结构
2. 在 `repositories/` 中创建对应的 Repository 接口和实现
3. 在 `services/` 中创建业务逻辑服务
4. 在 `routes/` 中创建 HTTP 处理器
5. 在 `migrations/` 中添加数据库迁移
6. 在 `main.go` 中注册依赖

### 修改数据库结构

1. 在 `models/` 中修改模型定义
2. 创建新的迁移文件：`migrations/YYYYMMDD_<description>.go`
3. 实现 `func (m *MigrationXXX) Up(db *gorm.DB) error` 方法
4. 在 `migrations/migrations.go` 中注册新迁移
5. 测试迁移：`./testing/run_api_tests.sh --migration`

### 添加新的定时任务

在 `main.go` 中使用 cron：

```go
// 在 housekeepingService 或 aggregationService 中添加
c.AddFunc(config.App.YourNewTime, func() {
    yourService.DoSomething()
})
```

### 添加新的认证提供商

1. 在 `config.yml` 中配置 OIDC：
   ```yaml
   security:
     oidc:
       - name: gitlab
         client_id: xxx
         client_secret: xxx
         endpoint: https://gitlab.com
   ```
2. 认证逻辑已实现在 `routes/login.go` 和 `middlewares/authenticate.go`

## 性能优化建议

1. **数据库索引**: 所有关键查询字段已建立索引（参考 `models/` 中的 `gorm` 标签）
2. **缓存**: 项目使用 `go-cache` 和 `golang-lru` 进行缓存
3. **并发池**: 使用 `alitto/pond` Worker 池处理批量任务
4. **数据库连接池**: 通过 `db.max_conn` 配置
5. **静态资源**: 已使用 Brotli 预压缩（`.br` 文件）

## 监控和日志

### Sentry 集成

```yaml
sentry:
  dsn: https://...@sentry.io/...
  enable_tracing: true
  sample_rate: 0.75
  sample_rate_heartbeats: 0.1  # 心跳请求采样率较低
```

### Prometheus 指标

```yaml
security:
  expose_metrics: true  # 启用后访问 /api/metrics
```

配置 Prometheus scrape config：
```yaml
scrape_configs:
  - job_name: 'wakapi'
    metrics_path: '/api/metrics'
    bearer_token: '<BASE64_HASHED_API_KEY>'
    static_configs:
      - targets: ['localhost:3000']
```

## Docker 部署

### 快速启动

```bash
# 生成密码盐
SALT="$(cat /dev/urandom | LC_ALL=C tr -dc 'a-zA-Z0-9' | fold -w 32 | head -n 1)"

# 运行容器
docker run -d \
  --init \
  -p 3000:3000 \
  -e "WAKAPI_PASSWORD_SALT=$SALT" \
  -v wakapi-data:/data \
  ghcr.io/muety/wakapi:latest
```

### Docker Compose

```bash
# 设置环境变量
export WAKAPI_PASSWORD_SALT=your-secret-salt
export WAKAPI_DB_PASSWORD=your-db-password

# 启动服务
docker compose up -d
```

参考 `compose.yml` 和 `Dockerfile`。

## 测试最佳实践

1. **单元测试**: 使用 `testify` 断言库，测试文件命名为 `*_test.go`
2. **API 测试**: 使用 Bruno 集合，确保测试顺序执行
3. **数据库测试**: 支持多种数据库的测试（SQLite、MySQL、PostgreSQL）
4. **测试数据**: 使用 `testing/data.sql` 种子数据

## 安全注意事项

1. **密码盐**: 必须设置 `WAKAPI_PASSWORD_SALT`（使用 Argon2id 哈希）
2. **HTTPS**: 生产环境必须使用 HTTPS（通过反向代理或 TLS 配置）
3. **CORS**: 已配置 CORS 中间件（`middlewares/security.go`）
4. **Rate Limiting**: 登录、注册、密码重置已启用速率限制
5. **Trusted Header 认证**: 使用前务必确保反向代理正确配置

## 贡献指南

参考以下资源：
- [贡献指南](https://github.com/muety/wakapi/wiki/Contributing)
- [设计目标](https://github.com/muety/wakapi/wiki/Design-Goals)

## 有用的链接

- 官网: https://wakapi.dev
- Swagger 文档: https://wakapi.dev/swagger-ui
- GitHub: https://github.com/muety/wakapi
- WakaTime 文档: https://wakatime.com/developers