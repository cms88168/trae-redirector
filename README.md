# HTTP 代理服务器

一个用Go语言实现的可配置HTTP代理服务器，支持根据请求路径转发到不同的远程服务器，路径与详细配置分离，Windows系统托盘支持动态切换配置。

## 功能特性

- 本地HTTP代理服务器
- 支持HTTP和HTTPS远程服务器
- **路径与配置分离**：routes定义路径匹配，configs定义详细配置，通过name关联
- **动态配置切换**：Windows系统托盘菜单按路由独立切换config，热重载无需重启
- **多路由支持**：根据请求路径匹配不同的远程服务器
- **完整的HTTP方法支持**：GET、POST、PUT、PATCH、DELETE等
- **请求体处理**：修改POST/PUT/PATCH的请求体数据
- **代理支持**：每个config可独立配置HTTP或SOCKS5代理
- **Token管理**：支持多Token池轮换、认证失败自动Fallback
  - **origin策略**：`use`(透传请求Token)、`add`(收集到池并用池Token)、`ignore`(只用池Token)
- 可配置的请求处理规则：
  - 请求头添加、删除、替换
  - URL路径修改（支持正则表达式）
  - 查询参数添加、删除、修改
  - 请求体替换、追加、转换（支持正则表达式）
  - **JSON专用操作**：添加、删除、替换、合并JSON字段（支持JSON路径）
- YAML配置文件（所有配置统一在一个文件中）
- 优雅的日志输出
- 连接池和超时处理
- **Windows系统托盘**：GUI子系统编译，不弹出控制台/WT窗口，日志写入文件或输出到终端

## 快速开始

### 1. 编译

```bash
# Linux/Mac
go build .

# Windows（必须使用 GUI 子系统，避免启动时弹出控制台/Windows Terminal 窗口）
go build -ldflags "-H windowsgui" .
```

### 2. 配置

编辑 `config.yaml` 文件。配置分为两部分：`routes`（路由路径）和 `configs`（详细配置），通过name关联：

```yaml
local_port: 8080
timeout: 30
debug: false

# 路由只定义路径和config引用
routes:
  - path_pattern: "^/api/"
    config: "api-service"
  - path_pattern: ".*"
    config: "default"

# 详细配置通过name区分
configs:
  - name: "api-service"
    remote_url: "https://api-backend.example.com"
    token:
      origin: "use"      # use | add | ignore
      tokens:
        - "token-1"
        - "token-2"
      header: "Authorization"
      prefix: "Bearer"
    rules:
      - type: "path"
        action: "replace"
        pattern: "^/api/(.*)"
        value: "/v2/$1"

  - name: "default"
    remote_url: "https://api.example.com"
    rules:
      - type: "header"
        action: "add"
        key: "X-Proxy-By"
        value: "my-proxy"
```

### 3. 运行

```bash
# Windows（双击启动：最小化到系统托盘，日志写入 trae-redirector.log）
.\trae-redirector.exe

# Windows（从终端启动：日志同时输出到终端和日志文件）
# 注意：从 cmd/PowerShell 启动时日志会实时显示在终端中
.\trae-redirector.exe

# Linux/Mac
./trae-redirector
```

**注意**：
- 程序会自动读取当前目录下的 `config.yaml` 文件，无需命令行参数
- Windows 版必须使用 `-ldflags "-H windowsgui"` 编译，否则会弹出控制台窗口

### 4. 测试

```bash
curl http://localhost:8080/api/test
```

## 配置说明

### 基本配置

- `local_port`: 本地监听端口 (1-65535)
- `timeout`: 请求超时时间（秒）
- `debug`: 调试模式，打印请求/响应详细信息

### 路由配置 (routes)

路由仅定义路径匹配和config引用：

- `path_pattern`: 路径匹配正则表达式
- `config`: 引用的configs中的name

### 详细配置 (configs)

每个config包含完整的转发配置：

- `name`: 配置名称（唯一标识，被routes引用）
- `remote_url`: 远程服务器URL（支持http和https）
- `proxy`: 代理配置（可选）
  - `type`: 代理类型（"http" 或 "socks5"）
  - `address`: 代理服务器地址
  - `username`: 代理认证用户名（可选）
  - `password`: 代理认证密码（可选）
- `token`: Token配置（可选，存在即启用）
  - `origin`: Token来源处理策略
    - `"use"`: 只使用来源请求中的Token，不替换（透传）
    - `"add"`（默认）: 将来源请求的Token收集到池中管理，同时使用池中Token替换请求头
    - `"ignore"`: 完全忽略来源请求的Token，只使用池中Token替换
  - `tokens`: Token列表
  - `header`: Token请求头名称（默认"Authorization"）
  - `prefix`: Token前缀（默认"Bearer"）
- `rules`: 请求处理规则列表

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

### Token配置

#### 基本Token配置（add模式 - 默认）
```yaml
token:
  origin: "add"      # 收集请求Token到池，并使用池中Token替换请求头
  tokens:
    - "token-abc-123"
    - "token-def-456"
    - "token-ghi-789"
  header: "Authorization"
  prefix: "Bearer"
```

#### use模式 - 透传请求Token
```yaml
token:
  origin: "use"      # 只使用来源请求中的Token，不替换
  tokens:
    - "seed-token"
  header: "Authorization"
  prefix: "Bearer"
```

适用场景：客户端自带Token，代理只需透传不干预。Token池仅作为记录使用。

#### ignore模式 - 只用池Token
```yaml
token:
  origin: "ignore"   # 忽略请求中的Token，强制使用池中Token
  tokens:
    - "managed-token-1"
    - "managed-token-2"
  header: "Authorization"
  prefix: "Bearer"
```

适用场景：客户端可能携带无效/过期Token，需要强制使用服务端管理的Token。

**Token轮换工作流程**（origin为add或ignore时）：
1. 第一次请求使用 token-abc-123
2. 如果认证失败（401/403），自动切换到 token-def-456
3. 再次失败则切换到 token-ghi-789
4. 所有Token用完后，下次请求从第一个Token重新开始

## Windows系统托盘

程序以 GUI 子系统编译（`-ldflags "-H windowsgui"`），Windows 不会为其创建控制台窗口，也不会拉起 Windows Terminal。启动后程序直接最小化到系统通知区域。

**日志输出方式**：

- **从终端启动**（cmd/PowerShell）：通过 `AttachConsole(ATTACH_PARENT_PROCESS)` 自动附加到父终端，日志同时输出到终端和日志文件
- **双击启动**（资源管理器）：日志仅写入 `trae-redirector.log` 文件，通过托盘菜单"打开日志"用记事本查看

**托盘功能**：

- **打开日志**：用记事本打开日志文件查看运行状态
- **动态切换配置**：每个路由显示当前使用的config，点击子菜单切换到其他config
- **热重载**：切换config后代理自动重载，无需重启

托盘菜单结构示例：
```
打开日志
─────────────
[^/nvidia] → nvidia
  ├─ ✓ nvidia
  ├─ deepseek
  └─ cmcc
[^/deepseek] → deepseek
  ├─ nvidia
  ├─ ✓ deepseek
  └─ cmcc
─────────────
退出
```

## 使用场景

1. **微服务代理**: 根据路径将请求路由到不同的微服务
2. **API网关**: 统一入口，根据不同API路径转发到后端服务
3. **请求修改**: 在转发前修改请求头、路径、参数或请求体
4. **代理转发**: 通过HTTP或SOCKS5代理转发请求
5. **调试工具**: 查看和修改HTTP请求/响应
6. **跨域解决方案**: 处理CORS问题
7. **多环境切换**: 通过托盘快速切换不同的API后端配置

## 技术栈

- Go 1.19+
- gopkg.in/yaml.v3 - YAML配置解析
- github.com/getlantern/systray - Windows系统托盘
- net/http - HTTP服务器和客户端

## 许可证

MIT License
