# Cursor2API

English | [简体中文](README.md)

A Go-based Cursor Web compatibility service supporting OpenAI Chat Completions, Anthropic Messages, OpenAI Responses, tools / function calling, Anthropic thinking mode, and Vision / OCR preprocessing.

[![Go Version](https://img.shields.io/badge/Go-1.24+-blue.svg)](https://golang.org)
[![License: PolyForm Noncommercial](https://img.shields.io/badge/License-PolyForm%20Noncommercial-orange.svg)](https://polyformproject.org/licenses/noncommercial/1.0.0/)

## ✨ Features

- ✅ OpenAI Chat Completions compatibility
- ✅ Anthropic Messages API compatibility (`/v1/messages`)
- ✅ OpenAI Responses API compatibility (`/v1/responses`)
- ✅ Streaming and non-streaming responses
- ✅ Tools / function calling support with compatibility parsing
- ✅ Anthropic thinking mode compatibility (`thinking` → `<thinking>` parsing + streaming blocks)
- ✅ Refusal interception, response sanitization, identity-probe mock responses
- ✅ Truncation detection + auto-continue for tool outputs
- ✅ Vision / OCR preprocessing (`ocr` or external `api` mode)
- ✅ Automatic Cursor Web authentication
- ✅ Clean web interface

## 🤖 Supported Models

- **Anthropic Claude**: claude-sonnet-4.6

## 🚀 Quick Start

### Requirements

- Go 1.24+
- For local OCR mode, install Tesseract runtime libraries (for example `libtesseract-dev`, `libleptonica-dev`, `tesseract-ocr-eng`, `tesseract-ocr-chi-sim`)

### Local Running Methods

#### Method 1: Manual Compile and Run (Recommended)

```bash
# Clone the project
git clone https://github.com/libaxuan/cursor2api-go.git
cd cursor2api-go

# Optional: copy the env template first
cp .env.example .env

# Install Go dependencies
go mod tidy

# If you want local OCR, install Tesseract runtime libraries first (Ubuntu/Debian)
sudo apt-get install -y libtesseract-dev libleptonica-dev tesseract-ocr tesseract-ocr-eng tesseract-ocr-chi-sim

# Build
go build -o cursor2api-go

# Run
./cursor2api-go
```

#### Method 2: Using go run

```bash
git clone https://github.com/libaxuan/cursor2api-go.git
cd cursor2api-go
go run .
```

#### Method 3: Using Makefile

```bash
make build
make run
```

The service will start at `http://localhost:8002`

## 🚀 Server Deployment Methods

### System Service Deployment (Linux)

1. **Compile and Move Binary**:
```bash
go build -o cursor2api-go
sudo mv cursor2api-go /usr/local/bin/
sudo chmod +x /usr/local/bin/cursor2api-go
```

2. **Create System Service File** `/etc/systemd/system/cursor2api-go.service`:
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

3. **Start Service**:
```bash
# Reload systemd configuration
sudo systemctl daemon-reload

# Enable auto-start on boot
sudo systemctl enable cursor2api-go

# Start service
sudo systemctl start cursor2api-go

# Check status
sudo systemctl status cursor2api-go
```

## 📡 API Usage

### List Models

```bash
curl -H "Authorization: Bearer 0000" http://localhost:8002/v1/models
```

### Anthropic Thinking Example

> `thinking` is request-level, not an environment variable and not a persistent YAML / `.env` setting.

```bash
curl -X POST http://localhost:8002/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: 0000" \
  -d '{
    "model": "claude-sonnet-4.6",
    "max_tokens": 1024,
    "thinking": {"type": "enabled", "budget_tokens": 2048},
    "messages": [{"role": "user", "content": "Think first, then answer this question"}]
  }'
```

### Non-Streaming Chat

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

### Streaming Chat

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

### Use in Third-Party Apps

In any app that supports custom OpenAI API (e.g., ChatGPT Next Web, Lobe Chat):

1. **API URL**: `http://localhost:8002`
2. **API Key**: `0000` (or custom)
3. **Model**: Choose from supported models

## ⚙️ Configuration

### Environment Variables

Recommended first step:

```bash
cp .env.example .env
```

If you prefer YAML, see `config.example.yaml`.

> `thinking` is not a config-file field; it is sent per request.

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8002` | Server port |
| `DEBUG` | `false` | Debug mode (shows detailed logs and route info when enabled) |
| `API_KEY` | `0000` | API authentication key (the default is only suitable for local development) |
| `MODELS` | `claude-sonnet-4.6,claude-sonnet-4-5-20250929,...` | Supported models (comma-separated; the first item is usually the default / preferred model) |
| `SYSTEM_PROMPT_INJECT` | `` | Extra instruction appended to the effective system prompt |
| `TIMEOUT` | `60` | Request timeout (seconds) |
| `MAX_INPUT_LENGTH` | `200000` | History trimming threshold (approximate characters) |
| `PROXY` | `` | Optional outbound proxy (http/https/socks5) |
| `USER_AGENT` | `Mozilla/5.0 ... Chrome/140...` | Override the default browser fingerprint UA |
| `VISION_ENABLED` | `false` | Enable image preprocessing / OCR |
| `VISION_MODE` | `ocr` | `ocr` (local Tesseract via gosseract) or `api` (external vision model) |
| `VISION_LANGUAGES` | `eng,chi_sim` | Local OCR languages (comma-separated) |
| `VISION_BASE_URL` | `https://api.openai.com/v1/chat/completions` | External vision API base URL |
| `VISION_API_KEY` | `` | Required when `VISION_MODE=api` |
| `VISION_MODEL` | `gpt-4o-mini` | External vision model name |

### Debug Mode

By default, the service runs in clean mode. To enable detailed logging:

**Option 1**: Modify `.env` file
```bash
DEBUG=true
```

**Option 2**: Use environment variable
```bash
DEBUG=true ./cursor2api-go
```

Debug mode displays:
- Detailed GIN route information
- Verbose request logs
- Browser fingerprint / request header details
- Retry and error handling details

### Troubleshooting

Having issues? Check the **[Troubleshooting Guide](TROUBLESHOOTING.md)** for solutions to common problems, including:
- 403 Access Denied errors
- Token fetch failures
- Connection timeouts
- Cloudflare blocking


## 🧪 Development

### Running Tests

```bash
# Run existing tests
go test ./...
```

### Running the local self-check

```bash
./scripts/local_self_check.sh

# or
make self-check
```

When `VISION_ENABLED=true && VISION_MODE=ocr`, the service also performs a startup OCR self-check. If Tesseract or the required language packs are missing, startup fails early with a clear error.

### Running the live smoke script

```bash
./scripts/e2e_smoke.sh

# or
make smoke
```

This script starts the real server locally and verifies:

- `/health`
- `/v1/models`
- `/v1/messages/count_tokens`
- identity-probe short-circuit paths for `/v1/messages`, `/v1/chat/completions`, and `/v1/responses`

### Running the real upstream matrix

```bash
./scripts/e2e_upstream_matrix.sh

# quick mode
MODE=quick ./scripts/e2e_upstream_matrix.sh

# or
make upstream-check
```

This script talks to the real Cursor Web upstream and classifies each check as:

- `PASS` — local proxy + upstream behavior matched expectations
- `WARN` — request succeeded, but upstream behavior was weaker/different than expected
- `FAIL` — local service, HTTP transport, or protocol framing failed

### Capability Matrix

See:
- `docs/API_CAPABILITIES.md`
- `docs/UPSTREAM_VALIDATION.md`

### Building

```bash
# Build executable
go build -o cursor2api-go

# Cross-compile (e.g., for Linux)
GOOS=linux GOARCH=amd64 go build -o cursor2api-go-linux
```

## 📁 Project Structure

```
cursor2api-go/
├── main.go              # Main entry point
├── config/              # Configuration management
├── compat/              # Protocol compatibility, OCR, and tool parsing
├── handlers/            # HTTP handlers
├── services/            # Cursor Web service layer
├── models/              # Data models
├── utils/               # Utility functions
├── middleware/          # Middleware
├── docs/                # Capability matrix and upstream validation docs
├── scripts/             # smoke / upstream matrix scripts
├── static/              # Static files
├── .env.example         # Environment variable template
├── config.example.yaml  # YAML config template
└── README.md            # Project documentation
```

## 🤝 Contributing

Contributions are welcome! Please follow these steps:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'feat: Add some AmazingFeature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

### Code Standards

- Follow [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- Format code with `gofmt`
- Check code with `go vet`
- Follow [Conventional Commits](https://conventionalcommits.org/) for commit messages

## 📄 License

This project is licensed under [PolyForm Noncommercial 1.0.0](https://polyformproject.org/licenses/noncommercial/1.0.0/).
Commercial use is not permitted. See the [LICENSE](LICENSE) file for details.

## ⚠️ Disclaimer

Please comply with the terms of service of related services when using this project.

---

⭐ If this project helps you, please give us a Star!
