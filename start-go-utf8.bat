@echo off
chcp 65001 >nul 2>&1
setlocal enabledelayedexpansion

::  Cursor2API启动脚本

echo.
echo =========================================
echo     🚀  Cursor2API启动器
echo =========================================
echo.

:: 检查Go是否安装
go version >nul 2>&1
if errorlevel 1 (
    echo ❌ Go 未安装，请先安装 Go 1.24 或更高版本
    echo 💡 安装方法: https://golang.org/dl/
    pause
    exit /b 1
)

:: 显示Go版本并检查版本号
for /f "tokens=3" %%i in ('go version') do set GO_VERSION=%%i
set GO_VERSION=!GO_VERSION:go=!

:: 检查Go版本是否满足要求 (需要 >= 1.24)
for /f "tokens=1,2 delims=." %%a in ("!GO_VERSION!") do (
    set MAJOR=%%a
    set MINOR=%%b
)
if !MAJOR! LSS 1 (
    echo ❌ Go 版本 !GO_VERSION! 过低，请安装 Go 1.24 或更高版本
    pause
    exit /b 1
)
if !MAJOR! EQU 1 if !MINOR! LSS 24 (
    echo ❌ Go 版本 !GO_VERSION! 过低，请安装 Go 1.24 或更高版本
    pause
    exit /b 1
)

echo ✅ Go 版本检查通过: !GO_VERSION!


:: 创建.env文件（如果不存在）
if not exist .env (
    echo 📝 创建默认 .env 配置文件...
    (
        echo # 服务器配置
        echo PORT=8002
        echo DEBUG=false
        echo.
        echo # API配置
        echo API_KEY=0000
        echo MODELS=claude-sonnet-4.6,claude-sonnet-4-5-20250929,claude-sonnet-4-20250514,claude-3-5-sonnet-20241022
        echo SYSTEM_PROMPT_INJECT=
        echo.
        echo # 请求配置
        echo TIMEOUT=60
        echo MAX_INPUT_LENGTH=200000
        echo USER_AGENT=Mozilla/5.0 ^(Windows NT 10.0; Win64; x64^) AppleWebKit/537.36 ^(KHTML, like Gecko^) Chrome/140.0.0.0 Safari/537.36
        echo.
        echo.
        echo # Vision / OCR配置
        echo VISION_ENABLED=false
        echo VISION_MODE=ocr
        echo VISION_LANGUAGES=eng,chi_sim
        echo VISION_BASE_URL=https://api.openai.com/v1/chat/completions
        echo VISION_API_KEY=
        echo VISION_MODEL=gpt-4o-mini
    ) > .env
    echo ✅ 默认 .env 文件已创建
) else (
    echo ✅ 配置文件 .env 已存在
)

:: 下载依赖
echo.
echo 📦 正在下载 Go 依赖...
go mod download
if errorlevel 1 (
    echo ❌ 依赖下载失败！
    pause
    exit /b 1
)

:: 构建应用
echo 🔨 正在编译 Go 应用...
go build -o cursor2api-go.exe .
if errorlevel 1 (
    echo ❌ 编译失败！
    pause
    exit /b 1
)

:: 检查构建是否成功
if not exist cursor2api-go.exe (
    echo ❌ 编译失败 - 可执行文件未找到！
    pause
    exit /b 1
)

echo ✅ 应用编译成功！

:: 显示服务信息
echo.
echo ✅ 准备就绪，正在启动服务...
echo.

:: 启动服务
cursor2api-go.exe

pause