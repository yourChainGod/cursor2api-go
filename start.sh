#!/bin/bash

#  Cursor2API启动脚本

set -e

# 定义颜色代码
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
WHITE='\033[1;37m'
NC='\033[0m' # No Color

# 打印标题
print_header() {
    echo ""
    echo -e "${CYAN}=========================================${NC}"
    echo -e "${WHITE}    🚀  Cursor2API启动器${NC}"
    echo -e "${CYAN}=========================================${NC}"
}

# 检查Go环境
check_go() {
    if ! command -v go &> /dev/null; then
        echo -e "${RED}❌ Go 未安装，请先安装 Go 1.24 或更高版本${NC}"
        echo -e "${YELLOW}💡 安装方法: https://golang.org/dl/${NC}"
        exit 1
    fi

    GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
    REQUIRED_VERSION="1.24"

    if [ "$(printf '%s\n' "$REQUIRED_VERSION" "$GO_VERSION" | sort -V | head -n1)" != "$REQUIRED_VERSION" ]; then
        echo -e "${RED}❌ Go 版本 $GO_VERSION 过低，请安装 Go $REQUIRED_VERSION 或更高版本${NC}"
        exit 1
    fi

    echo -e "${GREEN}✅ Go 版本检查通过: $GO_VERSION${NC}"
}

# 检查Node.js环境
check_nodejs() {
    if ! command -v node &> /dev/null; then
        echo -e "${RED}❌ Node.js 未安装，请先安装 Node.js 18 或更高版本${NC}"
        echo -e "${YELLOW}💡 安装方法: https://nodejs.org/${NC}"
        exit 1
    fi

    NODE_VERSION=$(node --version | sed 's/v//')
    REQUIRED_VERSION="18.0.0"

    if [ "$(printf '%s\n' "$REQUIRED_VERSION" "$NODE_VERSION" | sort -V | head -n1)" != "$REQUIRED_VERSION" ]; then
        echo -e "${RED}❌ Node.js 版本 $NODE_VERSION 过低，请安装 Node.js $REQUIRED_VERSION 或更高版本${NC}"
        exit 1
    fi

    echo -e "${GREEN}✅ Node.js 版本检查通过: $NODE_VERSION${NC}"
}

# 处理环境配置
setup_env() {
    if [ ! -f .env ]; then
        echo -e "${YELLOW}📝 创建默认 .env 配置文件...${NC}"
        cat > .env << EOF
# 服务器配置
PORT=8002
DEBUG=false

# API配置
API_KEY=0000
MODELS=claude-sonnet-4.6,claude-sonnet-4-5-20250929,claude-sonnet-4-20250514,claude-3-5-sonnet-20241022
SYSTEM_PROMPT_INJECT=

# 请求配置
TIMEOUT=60
MAX_INPUT_LENGTH=200000
USER_AGENT=Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36
UNMASKED_VENDOR_WEBGL=Google Inc. (Intel)
UNMASKED_RENDERER_WEBGL=ANGLE (Intel, Intel(R) UHD Graphics 620 Direct3D11 vs_5_0 ps_5_0, D3D11)

# Cursor配置
SCRIPT_URL=https://cursor.com/_next/static/chunks/pages/_app.js

# Vision / OCR配置
VISION_ENABLED=false
VISION_MODE=ocr
VISION_BASE_URL=https://api.openai.com/v1/chat/completions
VISION_API_KEY=
VISION_MODEL=gpt-4o-mini
EOF
        echo -e "${GREEN}✅ 默认 .env 文件已创建${NC}"
    else
        echo -e "${GREEN}✅ 配置文件 .env 已存在${NC}"
    fi
}

# 安装 Node 运行时依赖（用于 OCR / JS helper）
install_node_deps() {
    if [ -f package.json ]; then
        if [ ! -d node_modules ] || [ ! -f node_modules/tesseract.js/package.json ]; then
            echo -e "${BLUE}📦 正在安装 Node 运行时依赖...${NC}"
            npm install --omit=dev
        else
            echo -e "${GREEN}✅ Node 运行时依赖已就绪${NC}"
        fi
    fi
}

# 构建应用
build_app() {
    echo -e "${BLUE}📦 正在下载 Go 依赖...${NC}"
    go mod download

    install_node_deps

    echo -e "${BLUE}🔨 正在编译 Go 应用...${NC}"
    go build -o cursor2api-go .

    if [ ! -f cursor2api-go ]; then
        echo -e "${RED}❌ 编译失败！${NC}"
        exit 1
    fi

    echo -e "${GREEN}✅ 应用编译成功！${NC}"
}

# 显示服务信息
show_info() {
    echo ""
    echo -e "${GREEN}✅ 准备就绪，正在启动服务...${NC}"
    echo ""
}

# 启动服务器
start_server() {
    # 捕获中断信号
    trap 'echo -e "\n${YELLOW}⏹️  正在停止服务器...${NC}"; exit 0' INT

    ./cursor2api-go
}

# 主函数
main() {
    print_header
    check_go
    check_nodejs
    setup_env
    build_app
    show_info
    start_server
}

# 运行主函数
main