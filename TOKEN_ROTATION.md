# Token轮换和Fallback功能说明

## 概述

代理服务器现在支持路由级的Token池管理，可以配置多个Token进行轮换使用，并在认证失败时自动切换到下一个Token重试。

## 功能特性

### 1. 多Token轮换
- 每个路由可独立配置Token池
- Token按顺序循环使用
- 支持自定义Token头和前缀

### 2. 认证失败Fallback
- 自动检测401/403认证失败
- 失败时自动切换下一个Token
- 重试整个请求（包括所有规则处理）

### 3. 线程安全
- 每个路由独立的TokenManager
- 使用sync.Mutex保护状态
- 支持高并发请求

## 配置说明

### Token配置结构

```yaml
token:
  enabled: true              # 是否启用Token轮换
  tokens:                    # Token列表
    - "token-1"
    - "token-2"
    - "token-3"
  header: "Authorization"    # Token请求头名称（默认: Authorization）
  prefix: "Bearer"           # Token前缀（默认: Bearer）
```

### 配置字段说明

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| enabled | bool | 是 | - | 是否启用Token轮换 |
| tokens | []string | 是 | - | Token列表，至少一个 |
| header | string | 否 | "Authorization" | Token请求头名称 |
| prefix | string | 否 | "Bearer" | Token前缀，空字符串表示无前缀 |

## 工作流程

### 正常流程

```
请求到达 → 匹配路由 → 获取TokenManager
  → 使用当前Token（索引0） → 应用Token到请求头
  → 发送请求 → 认证成功 → 返回响应
```

### Fallback流程

```
请求到达 → 匹配路由 → 获取TokenManager
  → 使用当前Token（索引0） → 发送请求
  → 认证失败（401/403）
  → 记录失败 → 切换到下一个Token（索引1）
  → 重新克隆请求 → 重新应用规则
  → 应用新Token → 重试请求
  → 认证成功 → 返回响应
```

### Token循环

```
Token池: [Token-A, Token-B, Token-C]

请求1: 使用 Token-A (索引0) → 成功
请求2: 使用 Token-A (索引0) → 失败 → 切换到 Token-B
请求3: 使用 Token-B (索引1) → 成功
请求4: 使用 Token-B (索引1) → 失败 → 切换到 Token-C
请求5: 使用 Token-C (索引2) → 失败 → 切换到 Token-A (循环)
请求6: 使用 Token-A (索引0) → 成功
```

## 配置示例

### 示例1：基本Token轮换

```yaml
routes:
  - path_pattern: "^/api/"
    remote_url: "https://api-backend.example.com"
    token:
      enabled: true
      tokens:
        - "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.abc123"
        - "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.def456"
        - "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.ghi789"
      header: "Authorization"
      prefix: "Bearer"
```

**效果**：
- 请求头: `Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.abc123`
- 如果认证失败，自动切换到下一个Token

### 示例2：自定义Token头

```yaml
routes:
  - path_pattern: "^/internal/"
    remote_url: "http://internal-service:3000"
    token:
      enabled: true
      tokens:
        - "api-key-123456"
        - "api-key-789012"
      header: "X-API-Key"
      prefix: ""
```

**效果**：
- 请求头: `X-API-Key: api-key-123456`
- 无前缀，直接传递Token值

### 示例3：不使用Token

```yaml
routes:
  - path_pattern: "^/public/"
    remote_url: "https://public-api.example.com"
    # 不配置token字段，或enabled: false
```

**效果**：
- 不添加任何Token到请求头
- 正常转发请求

## 完整配置示例

```yaml
local_port: 8080
timeout: 30

routes:
  # API路由 - 使用3个Token轮换
  - path_pattern: "^/api/"
    remote_url: "https://api-backend.example.com"
    token:
      enabled: true
      tokens:
        - "token-abc-123"
        - "token-def-456"
        - "token-ghi-789"
      header: "Authorization"
      prefix: "Bearer"
    rules:
      - type: "path"
        action: "replace"
        pattern: "^/api/(.*)"
        value: "/v2/$1"
  
  # 内部服务 - 使用2个API Key
  - path_pattern: "^/internal/"
    remote_url: "http://internal-service:3000"
    token:
      enabled: true
      tokens:
        - "internal-key-1"
        - "internal-key-2"
      header: "X-Internal-Key"
      prefix: ""
    rules:
      - type: "header"
        action: "add"
        key: "X-Service"
        value: "proxy"
  
  # 公开路由 - 不使用Token
  - path_pattern: "^/public/"
    remote_url: "https://public-api.example.com"
    rules:
      - type: "header"
        action: "add"
        key: "X-Public"
        value: "true"
  
  # 默认路由 - 使用2个Token
  - path_pattern: ".*"
    remote_url: "https://api.example.com"
    token:
      enabled: true
      tokens:
        - "default-token-1"
        - "default-token-2"
      header: "X-Auth-Token"
      prefix: ""
```

## 日志输出示例

### 成功请求

```
收到请求: POST /api/users
匹配路由: ^/api/ -> https://api-backend.example.com
Token轮换已启用，Token数量: 3
使用Token索引: 0
转发到: https://api-backend.example.com/v2/users (尝试 1/3)
收到响应: 200 OK
响应完成: POST /api/users
```

### Fallback重试

```
收到请求: POST /api/users
匹配路由: ^/api/ -> https://api-backend.example.com
Token轮换已启用，Token数量: 3
使用Token索引: 0
转发到: https://api-backend.example.com/v2/users (尝试 1/3)
认证失败 (状态码: 401)，切换到下一个Token
已切换到Token索引: 1
使用Token索引: 1
转发到: https://api-backend.example.com/v2/users (尝试 2/3)
收到响应: 200 OK
响应完成: POST /api/users
```

### 所有Token失败

```
收到请求: POST /api/users
匹配路由: ^/api/ -> https://api-backend.example.com
Token轮换已启用，Token数量: 3
使用Token索引: 0
转发到: https://api-backend.example.com/v2/users (尝试 1/3)
认证失败 (状态码: 401)，切换到下一个Token
已切换到Token索引: 1
使用Token索引: 1
转发到: https://api-backend.example.com/v2/users (尝试 2/3)
认证失败 (状态码: 401)，切换到下一个Token
已切换到Token索引: 2
使用Token索引: 2
转发到: https://api-backend.example.com/v2/users (尝试 3/3)
认证失败 (状态码: 401)，切换到下一个Token
已切换到Token索引: 0
所有Token认证失败，共尝试 3 个Token
```

## 技术实现

### TokenManager

每个路由独立的Token管理器：

```go
type TokenManager struct {
    mu           sync.Mutex
    tokens       []string
    currentIndex int
    header       string
    prefix       string
    failureCount int
}
```

**核心方法**：
- `GetCurrentToken()`: 获取当前Token
- `ApplyTokenToRequest()`: 应用Token到请求头
- `SwitchToNext()`: 切换到下一个Token
- `RecordFailure()`: 记录失败

### 重试机制

```go
for attempt := 0; attempt < maxRetries; attempt++ {
    // 1. 克隆请求
    retryReq := cloneRequest(r, bodyBytes)
    
    // 2. 应用规则
    handler.ProcessRequest(retryReq)
    
    // 3. 应用Token
    tokenManager.ApplyTokenToRequest(retryReq)
    
    // 4. 发送请求
    resp, err := client.Do(retryReq)
    
    // 5. 检查认证失败
    if resp.StatusCode == 401 || resp.StatusCode == 403 {
        tokenManager.SwitchToNext()
        continue // 重试
    }
    
    // 6. 成功，跳出循环
    break
}
```

## 使用场景

### 场景1：API限流绕过
当API服务对单个Token有调用频率限制时，使用多个Token轮换可以分散请求。

```yaml
token:
  enabled: true
  tokens:
    - "token-1"  # 100次/分钟
    - "token-2"  # 100次/分钟
    - "token-3"  # 100次/分钟
  # 总计: 300次/分钟
```

### 场景2：Token失效容错
当Token可能随时失效时，准备多个备用Token自动切换。

```yaml
token:
  enabled: true
  tokens:
    - "primary-token"     # 主Token
    - "backup-token-1"    # 备用Token 1
    - "backup-token-2"    # 备用Token 2
```

### 场景3：多账号轮换
使用多个账号的Token轮流请求，实现负载均衡。

```yaml
token:
  enabled: true
  tokens:
    - "user1-token"
    - "user2-token"
    - "user3-token"
    - "user4-token"
```

## 注意事项

1. **Token安全**
   - Token以明文存储在配置文件中
   - 建议使用环境变量或密钥管理工具
   - 定期更换Token

2. **重试影响**
   - 每次重试都会重新应用所有规则
   - 重试会增加请求延迟
   - 注意幂等性问题

3. **性能考虑**
   - 每个路由独立的TokenManager，内存占用小
   - 线程安全，支持高并发
   - Token切换操作非常快速

4. **错误处理**
   - 网络错误不会触发重试
   - 只有401/403会触发Token切换
   - 所有Token失败后返回401给客户端

5. **状态持久化**
   - Token索引在内存中维护
   - 重启服务后从第一个Token开始
   - 不持久化失败计数

## 测试方法

### 测试Token轮换

```bash
# 发送多个请求，观察Token切换
for i in {1..10}; do
  curl -X POST http://localhost:8080/api/test \
    -H "Content-Type: application/json" \
    -d '{"test": "data"}'
  echo ""
done
```

### 模拟认证失败

配置一个无效的Token，观察Fallback行为：

```yaml
token:
  enabled: true
  tokens:
    - "invalid-token-1"
    - "invalid-token-2"
    - "valid-token"  # 第三个Token是有效的
```

**预期结果**：
- 第1次尝试：invalid-token-1 → 401
- 切换到Token 2
- 第2次尝试：invalid-token-2 → 401
- 切换到Token 3
- 第3次尝试：valid-token → 200 成功

## 与认证服务的区别

| 特性 | Token轮换 | 认证服务 |
|------|----------|---------|
| Token来源 | 配置文件预配置 | 动态获取 |
| 刷新机制 | 手动更换配置 | 自动刷新 |
| 适用场景 | 多个固定Token | Token会过期 |
| 实现复杂度 | 简单 | 复杂 |

如果需要动态Token刷新（如OAuth2），建议配合外部认证服务使用。
