# JSON请求体处理功能说明

## 概述

代理服务器现在支持完整的JSON请求体处理能力，包括JSON专用操作和JSON路径支持，可以智能地修改JSON结构而不会误替换字符串值。

## JSON专用操作

### 1. json_add - 添加JSON字段

在JSON对象中添加新字段，支持顶层字段和嵌套字段。

#### 添加顶层字段
```yaml
- type: "body"
  action: "json_add"
  key: "source"
  value: "proxy"
```

**效果：**
```json
// 原始请求体
{"user": "john", "action": "login"}

// 处理后
{"user": "john", "action": "login", "source": "proxy"}
```

#### 添加嵌套字段（使用JSON路径）
```yaml
- type: "body"
  action: "json_add"
  json_path: "$.metadata.source"
  value: "proxy"
```

**效果：**
```json
// 原始请求体
{"user": "john"}

// 处理后
{"user": "john", "metadata": {"source": "proxy"}}
```

### 2. json_remove - 删除JSON字段

从JSON对象中删除字段，支持顶层字段和嵌套字段。

#### 删除顶层字段
```yaml
- type: "body"
  action: "json_remove"
  key: "password"
```

**效果：**
```json
// 原始请求体
{"username": "john", "password": "secret", "email": "john@example.com"}

// 处理后
{"username": "john", "email": "john@example.com"}
```

#### 删除嵌套字段
```yaml
- type: "body"
  action: "json_remove"
  json_path: "$.user.password"
```

**效果：**
```json
// 原始请求体
{"user": {"name": "john", "password": "secret", "email": "john@example.com"}}

// 处理后
{"user": {"name": "john", "email": "john@example.com"}}
```

### 3. json_replace - 替换JSON字段值

替换JSON字段的值，只替换存在的字段，支持嵌套字段。

#### 替换顶层字段
```yaml
- type: "body"
  action: "json_replace"
  key: "status"
  value: "active"
```

**效果：**
```json
// 原始请求体
{"user": "john", "status": "pending"}

// 处理后
{"user": "john", "status": "active"}
```

#### 替换嵌套字段
```yaml
- type: "body"
  action: "json_replace"
  json_path: "$.user.status"
  value: "active"
```

**效果：**
```json
// 原始请求体
{"user": {"name": "john", "status": "pending"}}

// 处理后
{"user": {"name": "john", "status": "active"}}
```

### 4. json_merge - 合并JSON对象

将另一个JSON对象深度合并到当前请求体中。

```yaml
- type: "body"
  action: "json_merge"
  value: '{"metadata": {"source": "proxy"}, "timestamp": "2024-01-01"}'
```

**效果：**
```json
// 原始请求体
{"user": "john", "metadata": {"version": "1.0"}}

// 处理后
{
  "user": "john",
  "metadata": {
    "version": "1.0",
    "source": "proxy"
  },
  "timestamp": "2024-01-01"
}
```

## JSON路径语法

JSON路径用于精确定位JSON对象中的嵌套字段。

### 支持的路径格式

- `$.field` - 顶层字段
  ```yaml
  json_path: "$.username"
  ```

- `$.parent.child` - 嵌套字段
  ```yaml
  json_path: "$.user.name"
  ```

- `$.a.b.c.d` - 多层嵌套
  ```yaml
  json_path: "$.data.user.profile.email"
  ```

### 路径示例

```json
{
  "user": {
    "profile": {
      "name": "John",
      "email": "john@example.com"
    },
    "settings": {
      "theme": "dark"
    }
  }
}
```

对应的路径：
- `$.user.profile.name` → "John"
- `$.user.profile.email` → "john@example.com"
- `$.user.settings.theme` → "dark"

## 智能值类型推断

`json_add`和`json_replace`操作会自动推断值的类型：

### 字符串值
```yaml
- type: "body"
  action: "json_add"
  key: "source"
  value: "proxy"  # 字符串
```
结果：`"source": "proxy"`

### 数字值
```yaml
- type: "body"
  action: "json_add"
  key: "count"
  value: "123"  # 会被解析为数字
```
结果：`"count": 123`

### 布尔值
```yaml
- type: "body"
  action: "json_add"
  key: "active"
  value: "true"  # 会被解析为布尔值
```
结果：`"active": true`

### 对象值
```yaml
- type: "body"
  action: "json_add"
  key: "metadata"
  value: '{"source": "proxy", "version": "1.0"}'
```
结果：`"metadata": {"source": "proxy", "version": "1.0"}`

### 数组值
```yaml
- type: "body"
  action: "json_add"
  key: "tags"
  value: '["tag1", "tag2", "tag3"]'
```
结果：`"tags": ["tag1", "tag2", "tag3"]`

## 完整配置示例

```yaml
routes:
  - path_pattern: "^/api/"
    remote_url: "https://api-backend.example.com"
    rules:
      # 1. 添加时间戳
      - type: "body"
        action: "json_add"
        key: "proxy_timestamp"
        value: "2024-01-01T00:00:00Z"
      
      # 2. 添加嵌套的元数据
      - type: "body"
        action: "json_add"
        json_path: "$.metadata.source"
        value: "proxy"
      
      - type: "body"
        action: "json_add"
        json_path: "$.metadata.version"
        value: "1.0"
      
      # 3. 删除敏感字段
      - type: "body"
        action: "json_remove"
        json_path: "$.user.password"
      
      - type: "body"
        action: "json_remove"
        key: "secret_token"
      
      # 4. 替换状态值
      - type: "body"
        action: "json_replace"
        json_path: "$.user.status"
        value: "verified"
      
      # 5. 合并额外数据
      - type: "body"
        action: "json_merge"
        value: >
          {
            "tracking": {
              "proxy_id": "proxy-001",
              "forwarded_at": "2024-01-01T00:00:00Z"
            }
          }
```

## 实际应用场景

### 场景1：API网关添加元数据
```yaml
routes:
  - path_pattern: "^/api/"
    remote_url: "https://backend.example.com"
    rules:
      # 添加请求追踪信息
      - type: "body"
        action: "json_add"
        json_path: "$._meta.request_id"
        value: "req-12345"
      
      - type: "body"
        action: "json_add"
        json_path: "$._meta.proxy"
        value: "api-gateway"
```

### 场景2：删除敏感信息
```yaml
routes:
  - path_pattern: "^/submit/"
    remote_url: "https://processor.example.com"
    rules:
      # 删除信用卡号
      - type: "body"
        action: "json_remove"
        json_path: "$.payment.credit_card"
      
      # 删除CVV
      - type: "body"
        action: "json_remove"
        key: "cvv"
```

### 场景3：数据转换
```yaml
routes:
  - path_pattern: "^/transform/"
    remote_url: "https://converter.example.com"
    rules:
      # 替换旧字段名为新字段名
      - type: "body"
        action: "json_replace"
        json_path: "$.user.old_id"
        value: "new_id_value"
      
      # 添加转换标记
      - type: "body"
        action: "json_add"
        key: "transformed"
        value: "true"
```

## 错误处理

### 非JSON请求体
如果对非JSON格式的请求体使用JSON操作，会返回清晰的错误：
```
JSON解析失败: invalid character 'x' looking for beginning of value
```

### 路径不存在
使用`json_replace`时，如果路径不存在，操作会被跳过（不会报错）。

### 中间节点不是对象
```json
{"user": "john"}
```
尝试访问`$.user.name`会报错：
```
路径 $.user.name 的中间节点不是对象
```

## 与通用操作的区别

### 通用transform操作（基于正则）
```yaml
- type: "body"
  action: "transform"
  pattern: '"old_value"'
  value: '"new_value"'
```
**问题**：可能误替换字符串值中的内容

### JSON专用操作（智能处理）
```yaml
- type: "body"
  action: "json_replace"
  key: "old_value"
  value: "new_value"
```
**优势**：只替换键名，不会误替换字符串值

## 注意事项

1. **Content-Type**：确保请求的Content-Type为`application/json`
2. **JSON格式**：请求体必须是有效的JSON格式
3. **路径限制**：暂不支持数组索引路径（如`$.users[0].name`）
4. **值类型**：value字段会智能推断类型，如果需要字符串，确保不会被解析为其他类型
5. **性能**：JSON操作会解析和重新序列化JSON，大请求体会有性能开销

## 测试示例

### 测试json_add
```bash
curl -X POST http://localhost:8080/api/test \
  -H "Content-Type: application/json" \
  -d '{"user": "john"}'

# 转发到远程服务器的请求体：
# {"user": "john", "proxy_timestamp": "2024-01-01", "metadata": {"source": "proxy"}}
```

### 测试json_remove
```bash
curl -X POST http://localhost:8080/api/test \
  -H "Content-Type: application/json" \
  -d '{"user": "john", "password": "secret"}'

# 转发到远程服务器的请求体：
# {"user": "john"}  # password字段已被删除
```

### 测试json_merge
```bash
curl -X POST http://localhost:8080/api/test \
  -H "Content-Type: application/json" \
  -d '{"user": "john"}'

# 转发到远程服务器的请求体：
# {"user": "john", "extra_field": "value", "nested": {"key": "data"}}
```
