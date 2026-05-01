# HTTP 代理服务器

一个用Go语言实现的可配置HTTP代理服务器，支持根据请求路径转发到不同的远程服务器，每个路由有独立的配置和规则。

## 功能特性

- 本地HTTP代理服务器
- 支持HTTP和HTTPS远程服务器
- **多路由支持**：根据请求路径匹配不同的远程服务器
- **完整的HTTP方法支持**：GET、POST、PUT、PATCH、DELETE等
- **请求体处理**：修改POST/PUT/PATCH的请求体数据
- **代理支持**：每个路由可独立配置HTTP或SOCKS5代理
- **Token轮换**：支持多Token池轮换和认证失败自动Fallback
- 每个路由独立的配置：
  - 独立的远程服务器URL
  - 独立的代理配置
  - 独立的请求处理规则
- 可配置的请求处理规则：
  - 请求头添加、删除、替换
  - URL路径修改（支持正则表达式）
  - 查询参数添加、删除、修改
  - 请求体替换、追加、转换（支持正则表达式）
  - **JSON专用操作**：添加、删除、替换、合并JSON字段（支持JSON路径）
- YAML配置文件
- 优雅的日志输出
- 连接池和超时处理

## 快速开始

### 1. 编译

```bash
go build .
```

### 2. 配置

编辑 `config.yaml` 文件：

```yaml
local_port: 8080
timeout: 30

# 路由配置
routes:
  # API路由
  - path_pattern: "^/api/"
    remote_url: "https://api-backend.example.com"
    rules:
      - type: "path"
        action: "replace"
        pattern: "^/api/(.*)"
        value: "/v2/$1"
  
  # 默认路由（必须）
  - path_pattern: ".*"
    remote_url: "https://api.example.com"
    rules:
      - type: "header"
        action: "add"
        key: "X-Proxy-By"
        value: "my-proxy"
```

### 3. 运行

```bash
# Windows
.\trae-redirector.exe

# Linux/Mac
./trae-redirector
```

**注意**：程序会自动读取当前目录下的 `config.yaml` 文件，无需命令行参数。

### 4. 测试

```bash
curl http://localhost:8080/api/test
```

## 配置说明

### 基本配置

- `local_port`: 本地监听端口 (1-65535)
- `timeout`: 请求超时时间（秒）

### 路由配置

程序支持配置多个路由，每个路由包含：

- `path_pattern`: 路径匹配正则表达式
- `remote_url`: 远程服务器URL（支持http和https）
- `proxy`: 代理配置（可选）
  - `type`: 代理类型（"http" 或 "socks5"）
  - `address`: 代理服务器地址
  - `username`: 代理认证用户名（可选）
  - `password`: 代理认证密码（可选）
- `token`: Token配置（可选）
  - `enabled`: 是否启用Token轮换
  - `tokens`: Token列表
  - `header`: Token请求头名称（默认"Authorization"）
  - `prefix`: Token前缀（默认"Bearer"）
- `rules`: 该路由的请求处理规则列表

**重要**：必须配置一个默认路由（`path_pattern: ".*"`）来处理未匹配的请求。

### 规则配置

每个规则包含以下字段：

- `type`: 规则类型
  - `header`: 请求头处理
  - `path`: URL路径处理
  - `query`: 查询参数处理
  - `body`: 请求体处理（POST/PUT/PATCH）

- `action`: 操作类型
  - `add`: 添加
  - `remove`: 删除
  - `replace`: 替换
  - `append`: 追加（仅body类型）
  - `transform`: 转换，使用正则表达式替换（仅body类型）

- `key`: 键名（header和query类型使用）
- `value`: 值
- `pattern`: 正则表达式模式（path替换和body转换时使用）

### 规则示例

#### 添加请求头
```yaml
- type: "header"
  action: "add"
  key: "X-Custom-Header"
  value: "custom-value"
```

#### 替换路径
```yaml
- type: "path"
  action: "replace"
  pattern: "^/api/(.*)"
  value: "/v2/$1"
```

#### 添加查询参数
```yaml
- type: "query"
  action: "add"
  key: "token"
  value: "abc123"
```

#### 修改请求体（替换JSON字段）
```yaml
- type: "body"
  action: "transform"
  pattern: '"old_field"'
  value: '"new_field"'
```

#### 完全替换请求体
```yaml
- type: "body"
  action: "replace"
  value: '{"modified": true}'
```

#### 追加请求体内容
```yaml
- type: "body"
  action: "append"
  value: ',"extra":"data"'
```

### JSON专用操作

#### 添加JSON字段
```yaml
# 添加顶层字段
- type: "body"
  action: "json_add"
  key: "source"
  value: "proxy"

# 添加嵌套字段（使用JSON路径）
- type: "body"
  action: "json_add"
  json_path: "$.metadata.source"
  value: "proxy"
```

#### 删除JSON字段
```yaml
# 删除顶层字段
- type: "body"
  action: "json_remove"
  key: "sensitive_field"

# 删除嵌套字段
- type: "body"
  action: "json_remove"
  json_path: "$.user.password"
```

#### 替换JSON字段值
```yaml
# 替换顶层字段
- type: "body"
  action: "json_replace"
  key: "status"
  value: "active"

# 替换嵌套字段
- type: "body"
  action: "json_replace"
  json_path: "$.user.status"
  value: "active"
```

#### 合并JSON对象
```yaml
- type: "body"
  action: "json_merge"
  value: '{"extra_field": "value", "nested": {"key": "data"}}'
```

**JSON路径语法**：
- `$.field` - 顶层字段
- `$.user.name` - 嵌套字段
- `$.data.id` - 多层嵌套

### 代理配置示例

#### HTTP代理
```yaml
proxy:
  type: "http"
  address: "http://proxy.example.com:8080"
  username: "user"  # 可选
  password: "pass"  # 可选
```

#### SOCKS5代理
```yaml
proxy:
  type: "socks5"
  address: "socks5://proxy.example.com:1080"
```

### Token轮换配置

#### 基本Token配置
```yaml
token:
  enabled: true
  tokens:
    - "token-abc-123"
    - "token-def-456"
    - "token-ghi-789"
  header: "Authorization"
  prefix: "Bearer"
```

**工作流程**：
1. 第一次请求使用 token-abc-123
2. 如果认证失败（401/403），自动切换到 token-def-456
3. 再次失败则切换到 token-ghi-789
4. 所有Token用完后，下次请求从第一个Token重新开始

#### 自定义Token头
```yaml
token:
  enabled: true
  tokens:
    - "api-key-123"
    - "api-key-456"
  header: "X-API-Key"
  prefix: ""  # 无前缀
```

**Fallback机制**：
- 当收到401或403状态码时，自动切换下一个Token
- 每个路由独立的Token池，互不影响
- 线程安全，支持并发请求

## 使用场景

1. **微服务代理**: 根据路径将请求路由到不同的微服务
2. **API网关**: 统一入口，根据不同API路径转发到后端服务
3. **请求修改**: 在转发前修改请求头、路径、参数或请求体
4. **代理转发**: 通过HTTP或SOCKS5代理转发请求
5. **调试工具**: 查看和修改HTTP请求/响应
6. **跨域解决方案**: 处理CORS问题

## 技术栈

- Go 1.19+
- gopkg.in/yaml.v3 - YAML配置解析
- net/http - HTTP服务器和客户端

## 许可证

MIT License
