# Cursor2API

[English](README_EN.md) | 简体中文

一个以 Go 实现的 Cursor Web 协议兼容服务，支持 OpenAI Chat Completions、Anthropic Messages、OpenAI Responses、tools / function calling、Anthropic thinking 模式、以及 Vision API 预处理。

[![Go Version](https://img.shields.io/badge/Go-1.24+-blue.svg)](https://golang.org)
[![License: PolyForm Noncommercial](https://img.shields.io/badge/License-PolyForm%20Noncommercial-orange.svg)](https://polyformproject.org/licenses/noncommercial/1.0.0/)

## 🧬 项目来源

本项目基于以下开源项目改造而来，在此致谢：

| 项目 | 贡献 |
|---|---|
| [libaxuan/cursor2api-go](https://github.com/libaxuan/cursor2api-go) | 主要基础代码，Go 实现的 Cursor Web 协议兼容层 |
| [7836246/cursor2api](https://github.com/7836246/cursor2api) | 借鉴：阶梯式截断恢复、续写去重、工具模式拒绝引导文本、Token 估算优化 |
| [510myRday/Cursor-Toolbox](https://github.com/510myRday/Cursor-Toolbox) | 借鉴：角色扩展注入反拒绝策略 |
| [highkay/cursor2api-go](https://github.com/highkay/cursor2api-go) | 参考：reasoning_content 暴露方案 |

## 🆕 相较上游的改进

- ✅ **真流式输出**：改造 `streamAnthropic` / `streamOpenAI`，从上游逐帧转发，不再先攒完整响应再发
- ✅ **增量流式解析器**：新增 `StreamResponseParser`，实时解析 thinking / tool 块
- ✅ **UTF-8 安全切片**：修复所有按字节切 UTF-8 字符串导致的乱码问题
- ✅ **阶梯式截断恢复**：引导模型分块写入，降级才用传统续写
- ✅ **续写智能去重**：拼接时自动检测并去除重复段落
- ✅ **并发安全**：HeaderGenerator 加 sync.Mutex，防止并发 panic
- ✅ **SSE goroutine 泄漏修复**：ctx cancel 时主动关闭 resp.Body
- ✅ **缓冲式流式 sanitize**：攒 200 字符再做正则清洗，防止碎片误判
- ✅ **去掉本地 OCR**：移除 tesseract/gosseract 依赖，仅保留外部 Vision API 模式
- ✅ **自动生成配置**：首次启动若无配置文件，自动生成含 `sk-` 随机密钥的 `config.yaml`
- ✅ **启动日志显示完整密钥**：方便复制使用

## ✨ 特性

- 🔄 **API 兼容**: 兼容 OpenAI Chat Completions、Anthropic Messages、OpenAI Responses 接口
- ⚡ **真流式输出**: 上游逐帧转发，首字延迟大幅降低
- 🔐 **安全认证**: 支持 API Key 认证，首次启动自动生成
- 🌐 **多模型支持**: 支持多种 Claude 模型名映射
- 🛡️ **反拒绝策略**: 多层次拒绝检测、重试、响应清洗
- 📊 **健康检查**: 内置健康检查接口

## ✨ 功能特性

- ✅ 兼容 OpenAI Chat Completions
- ✅ 兼容 Anthropic Messages API（`/v1/messages`）
- ✅ 兼容 OpenAI Responses API（`/v1/responses`，适配 Cursor IDE Agent 模式）
- ✅ 支持流式和非流式响应（真流式，非缓冲后发送）
- ✅ 支持 Anthropic thinking 模式（`thinking` → `<thinking>` 增量解析与 streaming block 输出）
- ✅ 支持 tools / function calling 与工具调用解析
- ✅ 内置拒绝拦截、响应清洗、tool_choice=any 兜底重试
- ✅ 支持身份探针拦截与模拟 Claude 响应
- ✅ 阶梯式截断恢复与续写智能去重
- ✅ 自动处理 Cursor Web 认证
- ✅ 首次启动自动生成 `config.yaml`（含随机 `sk-` API Key）
- ✅ 支持外部 Vision API 图片预处理
- ✅ 简洁的 Web 界面

## 🤖 支持的模型

- **Anthropic Claude**: claude-sonnet-4.6（及所有配置的模型）
- **Thinking 变体**: 每个模型自动生成 `*-thinking` 变体（如 `claude-sonnet-4.6-thinking`），请求时自动启用扩展推理

## 🚀 快速开始

### 环境要求

- Go 1.24+（从源码编译时需要）
- 或直接下载 [Release 预编译二进制](https://github.com/yourChainGod/cursor2api-go/releases)

### 方法一：直接使用预编译二进制（推荐）

从 [Releases](https://github.com/yourChainGod/cursor2api-go/releases) 下载对应平台二进制：

| 平台 | 文件名 |
|---|---|
| Linux x86_64 | `cursor2api-linux-amd64` |
| Windows x86_64 | `cursor2api-windows-amd64.exe` |
| macOS Apple Silicon | `cursor2api-darwin-arm64` |

首次运行会自动生成 `config.yaml`，包含随机生成的 `sk-` API Key，直接可用。

### 方法二：手动编译运行

```bash
# 克隆项目
git clone https://github.com/yourChainGod/cursor2api-go.git
cd cursor2api-go

# 安装 Go 依赖
go mod tidy

# 编译
go build -o cursor2api-go .

# 运行（首次运行自动生成 config.yaml）
./cursor2api-go
```

### 方法三：使用 Docker（推荐生产部署）

```bash
docker compose up -d
```

详见 `docker-compose.yml`。

服务将在 `http://localhost:8002` 启动

## 🚀 服务器部署方式

### 系统服务部署（Linux）

1. **编译并移动二进制文件**:
```bash
go build -o cursor2api-go
sudo mv cursor2api-go /usr/local/bin/
sudo chmod +x /usr/local/bin/cursor2api-go
```

2. **创建系统服务文件** `/etc/systemd/system/cursor2api-go.service`:
```ini
[Unit]
Description=Cursor2API Service
After=network.target

[Service]
Type=simple
User=your-user
WorkingDirectory=/home/your-user/cursor2api-go
ExecStart=/usr/local/bin/cursor2api-go
Restart=always
Environment=API_KEY=your-secret-key
Environment=PORT=8002

[Install]
WantedBy=multi-user.target
```

3. **启动服务**:
```bash
# 重载 systemd 配置
sudo systemctl daemon-reload

# 启用开机自启
sudo systemctl enable cursor2api-go

# 启动服务
sudo systemctl start cursor2api-go

# 查看状态
sudo systemctl status cursor2api-go
```

## 📡 API 使用

### 获取模型列表

```bash
curl -H "Authorization: Bearer 0000" http://localhost:8002/v1/models
```

### Anthropic Messages API

```bash
curl -X POST http://localhost:8002/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: 0000" \
  -d '{
    "model": "claude-sonnet-4.6",
    "max_tokens": 1024,
    "messages": [{"role": "user", "content": "hello"}]
  }'
```

### Anthropic Thinking 示例

> `thinking` 是请求级参数，不是环境变量，也不是 YAML / `.env` 常驻配置。

```bash
curl -X POST http://localhost:8002/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: 0000" \
  -d '{
    "model": "claude-sonnet-4.6",
    "max_tokens": 1024,
    "thinking": {"type": "enabled", "budget_tokens": 2048},
    "messages": [{"role": "user", "content": "请先思考，再回答这个问题"}]
  }'
```

### OpenAI Responses API

```bash
curl -X POST http://localhost:8002/v1/responses \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer 0000" \
  -d '{
    "model": "claude-sonnet-4-20250514",
    "input": "list files in the project",
    "stream": false
  }'
```

### 非流式聊天

```bash
curl -X POST http://localhost:8002/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer 0000" \
  -d '{
    "model": "claude-sonnet-4.6",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": false
  }'
```

### 流式聊天

```bash
curl -X POST http://localhost:8002/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer 0000" \
  -d '{
    "model": "claude-sonnet-4.6",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": true
  }'
```

### 在第三方应用中使用

在任何支持自定义 OpenAI API 的应用中（如 ChatGPT Next Web、Lobe Chat 等）：

1. **API 地址**: `http://localhost:8002`
2. **API 密钥**: `0000`（或自定义）
3. **模型**: 选择支持的模型之一

## ⚙️ 配置说明

### 环境变量

推荐先复制模板：

```bash
cp .env.example .env
```

如果你偏好 YAML，也可以参考：`config.example.yaml`

> `thinking` 不是配置文件项，而是每次请求单独携带的参数。

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| `PORT` | `8002` | 服务器端口 |
| `DEBUG` | `false` | 调试模式（启用后显示详细日志和路由信息） |
| `API_KEY` | `0000` | API 认证密钥（默认值仅建议本地开发使用） |
| `MODELS` | `claude-sonnet-4.6,claude-sonnet-4-5-20250929,...` | 支持的模型列表（逗号分隔，第一项通常作为默认 / 推荐模型） |
| `SYSTEM_PROMPT_INJECT` | `` | 追加到有效 system prompt 的额外指令 |
| `TIMEOUT` | `60` | 请求超时时间（秒） |
| `MAX_INPUT_LENGTH` | `200000` | 历史消息裁剪阈值（按字符近似） |
| `PROXY` | `` | 可选代理（支持 http/https/socks5） |
| `USER_AGENT` | `Mozilla/5.0 ... Chrome/140...` | 覆盖默认浏览器指纹 UA |
| `VISION_ENABLED` | `false` | 是否启用图片预处理（通过外部 Vision API） |
| `VISION_MODE` | `api` | 图片处理模式（仅支持 `api`，本地 OCR 已移除） |
| `VISION_BASE_URL` | `https://api.openai.com/v1/chat/completions` | 外部视觉 API 地址 |
| `VISION_API_KEY` | `` | 外部视觉 API Key（`VISION_ENABLED=true` 时必填） |
| `VISION_MODEL` | `gpt-4o-mini` | 外部视觉模型名 |
| `ENABLE_THINKING` | `false` | 全局启用扩展推理模式（也可通过请求参数或 `-thinking` 模型名按请求控制） |

### 图片预处理

Go 版支持将图片输入在发送到 Cursor Web 前通过外部 Vision API 做预处理：

示例：

```bash
VISION_ENABLED=true \
VISION_MODE=api \
VISION_API_KEY=your-key \
./cursor2api-go
```

### 调试模式

默认情况下，服务以简洁模式运行。如需启用详细日志：

**方式 1**: 修改 `.env` 文件
```bash
DEBUG=true
```

**方式 2**: 使用环境变量
```bash
DEBUG=true ./cursor2api-go
```

调试模式会显示：
- 详细的 GIN 路由信息
- 每个请求的详细日志
- 浏览器指纹 / 请求头配置
- 重试与错误处理信息

### 故障排除

遇到问题？查看 **[故障排除指南](TROUBLESHOOTING.md)** 了解常见问题的解决方案，包括：
- 403 Access Denied 错误
- Token 获取失败
- 连接超时
- Cloudflare 拦截


## 🧪 开发

### 运行测试

```bash
# 运行现有测试
go test ./...
```

### 运行本地自检

```bash
./scripts/local_self_check.sh

# 或
make self-check
```

### 运行 live smoke

```bash
./scripts/e2e_smoke.sh

# 或
make smoke
```

该脚本会：
- 启动真实服务进程
- 验证 `/health`
- 验证 `/v1/models`
- 验证 `/v1/messages/count_tokens`
- 验证 `/v1/messages` / `/v1/chat/completions` / `/v1/responses` 的 identity-probe 流式与非流式短路

### 运行真实 upstream matrix

```bash
./scripts/e2e_upstream_matrix.sh

# 快速模式
MODE=quick ./scripts/e2e_upstream_matrix.sh

# 或
make upstream-check
```

该脚本会直接命中真实 Cursor Web 上游，并把结果区分为：
- `PASS`：本地代理 + 上游行为都符合预期
- `WARN`：请求成功，但上游行为未完全按预期配合
- `FAIL`：本地服务、HTTP、或协议成形失败

### 能力矩阵

详见：
- `docs/API_CAPABILITIES.md`
- `docs/UPSTREAM_VALIDATION.md`

### 构建项目

```bash
# 构建可执行文件
go build -o cursor2api-go

# 交叉编译 (例如 Linux)
GOOS=linux GOARCH=amd64 go build -o cursor2api-go-linux
```

## 📁 项目结构

```
cursor2api-go/
├── main.go              # 主程序入口
├── config/              # 配置管理
├── compat/              # 协议兼容层与 OCR / tool parser
├── handlers/            # HTTP 处理器
├── services/            # Cursor Web 服务层
├── models/              # 数据模型
├── utils/               # 工具函数
├── middleware/          # 中间件
├── docs/                # 能力矩阵 / 上游验证说明
├── scripts/             # smoke / upstream matrix 脚本
├── static/              # 静态文档页
├── .env.example         # 环境变量模板
├── config.example.yaml  # YAML 配置模板
└── README.md            # 项目说明
```

## 🤝 贡献指南

欢迎贡献代码！请遵循以下步骤：

1. Fork 本仓库
2. 创建功能分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'feat: Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 开启 Pull Request

### 代码规范

- 遵循 [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- 使用 `gofmt` 格式化代码
- 使用 `go vet` 检查代码
- 提交信息遵循 [Conventional Commits](https://conventionalcommits.org/) 规范

## 📄 许可证

本项目采用 [PolyForm Noncommercial 1.0.0](https://polyformproject.org/licenses/noncommercial/1.0.0/) 许可证。
禁止商业用途。查看 [LICENSE](LICENSE) 文件了解详情。

## ⚠️ 免责声明

使用本项目时请遵守相关服务的使用条款。

---

⭐ 如果这个项目对您有帮助，请给我们一个 Star！
