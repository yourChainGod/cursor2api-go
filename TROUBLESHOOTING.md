# 故障排除指南

## 403 Access Denied 错误

### 问题描述
在使用一段时间后,服务突然开始返回 `403 Access Denied` 错误:
```
ERRO[0131] Cursor API returned non-OK status             status_code=403
ERRO[0131] Failed to create chat completion              error="{\"error\":\"Access denied\"}"
```

### 原因分析
2. **频率限制**: 短时间内发送过多请求触发了 Cursor API 的速率限制
3. **重复 Token**: 使用相同的 token 进行多次请求被识别为异常行为

### 解决方案

#### 1. 已实施的自动修复
最新版本已经包含以下改进:

- **动态浏览器指纹**: 每次请求使用真实且随机的浏览器指纹信息
  - 根据操作系统自动选择合适的平台配置 (Windows/macOS/Linux)
  - 随机 Chrome 版本 (120-130)
  - 随机语言设置和 Referer
  - 真实的 User-Agent 和 sec-ch-ua headers
- **自动重试机制**: 遇到 403 错误时自动清除缓存并重试(最多 2 次)
- **指纹刷新**: 403 错误时自动刷新浏览器指纹配置
- **错误恢复**: 失败时自动清除缓存,确保下次请求使用新 token
- **指数退避**: 重试时使用递增的等待时间

#### 2. 手动解决步骤
如果问题持续存在:

1. **重启服务**:
   ```bash
   # 停止当前服务 (Ctrl+C)
   # 重新启动
   ./cursor2api-go
   ```

2. **检查日志**:
   查看是否有以下日志:
   - `Received 403 Access Denied, clearing token cache and retrying...` - 自动重试

3. **等待冷却期**:
   如果频繁遇到 403 错误,建议等待 5-10 分钟后再使用

4. **检查网络**:
   确保能够访问 `https://cursor.com`

#### 3. 预防措施

1. **控制请求频率**: 避免在短时间内发送大量请求
2. **监控日志**: 注意 403 / Cloudflare / 浏览器指纹刷新日志
3. **合理配置超时**: 在 `.env` 文件中设置合理的超时时间

### 配置建议

在 `.env` 文件中:
```bash
TIMEOUT=120  # 增加超时时间,避免频繁重试
MAX_INPUT_LENGTH=100000  # 限制输入长度,减少请求大小
```

### 调试模式

如果需要查看详细的调试信息,可以启用调试模式:
```bash
# 方式 1: 修改 .env 文件
DEBUG=true

# 方式 2: 使用环境变量
DEBUG=true ./cursor2api-go
```

这将显示:
- 每次请求的关键请求头与响应状态
- 请求的 payload 大小
- 重试次数
- 详细的错误信息

## 其他常见问题

### Cloudflare 403 错误
如果看到 `Cloudflare 403` 错误,说明请求被 Cloudflare 防火墙拦截。这通常是因为:
- IP 被标记为可疑
- User-Agent 不匹配
- 缺少必要的浏览器指纹

**解决方案**: 检查 `.env` 文件中的 `USER_AGENT` 配置，必要时启用 DEBUG 观察请求头。

### 连接超时
如果频繁出现连接超时:
1. 检查网络连接
2. 增加 `.env` 文件中的 `TIMEOUT` 配置值
3. 检查防火墙设置

### Token 获取失败

## 联系支持

如果问题仍未解决,请提供以下信息:
1. 完整的错误日志
2. `.env` 文件配置（隐藏敏感信息如 `API_KEY`）
3. 使用的 Go 版本与 Tesseract 安装情况
4. 操作系统信息
