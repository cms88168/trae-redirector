package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// RequestHandler 请求处理器
type RequestHandler struct {
	rules []Rule
	body  []byte // 缓存的请求体
}

// NewRequestHandler 创建请求处理器
func NewRequestHandler(rules []Rule) *RequestHandler {
	return &RequestHandler{
		rules: rules,
	}
}

// SetBody 设置请求体缓存
func (h *RequestHandler) SetBody(body []byte) {
	h.body = body
}

// GetBody 获取处理后的请求体
func (h *RequestHandler) GetBody() []byte {
	return h.body
}

// ProcessRequest 处理HTTP请求，应用所有配置规则
func (h *RequestHandler) ProcessRequest(req *http.Request) error {
	for i, rule := range h.rules {
		if err := h.applyRule(req, rule); err != nil {
			return fmt.Errorf("应用规则 %d 失败: %w", i, err)
		}
	}
	return nil
}

// applyRule 应用单个规则
func (h *RequestHandler) applyRule(req *http.Request, rule Rule) error {
	switch rule.Type {
	case "header":
		return h.processHeader(req, rule)
	case "path":
		return h.processPath(req, rule)
	case "query":
		return h.processQuery(req, rule)
	case "body":
		return h.processBody(req, rule)
	default:
		return fmt.Errorf("不支持的规则类型: %s", rule.Type)
	}
}

// processHeader 处理请求头规则
func (h *RequestHandler) processHeader(req *http.Request, rule Rule) error {
	switch rule.Action {
	case "add":
		req.Header.Add(rule.Key, rule.Value)
	case "remove":
		req.Header.Del(rule.Key)
	case "replace":
		req.Header.Set(rule.Key, rule.Value)
	default:
		return fmt.Errorf("不支持的header操作: %s", rule.Action)
	}
	return nil
}

// processPath 处理路径规则
func (h *RequestHandler) processPath(req *http.Request, rule Rule) error {
	switch rule.Action {
	case "replace":
		if rule.Pattern == "" {
			return fmt.Errorf("路径替换需要提供pattern")
		}

		// 编译正则表达式
		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return fmt.Errorf("正则表达式编译失败: %w", err)
		}

		// 执行替换
		newPath := re.ReplaceAllString(req.URL.Path, rule.Value)
		req.URL.Path = newPath

	case "add":
		// 添加路径前缀
		if !strings.HasPrefix(rule.Value, "/") {
			rule.Value = "/" + rule.Value
		}
		req.URL.Path = rule.Value + req.URL.Path

	case "remove":
		// 移除路径前缀
		if rule.Key != "" && strings.HasPrefix(req.URL.Path, rule.Key) {
			req.URL.Path = strings.TrimPrefix(req.URL.Path, rule.Key)
		}

	default:
		return fmt.Errorf("不支持的path操作: %s", rule.Action)
	}

	return nil
}

// processQuery 处理查询参数规则
func (h *RequestHandler) processQuery(req *http.Request, rule Rule) error {
	query := req.URL.Query()

	switch rule.Action {
	case "add":
		query.Add(rule.Key, rule.Value)
	case "remove":
		query.Del(rule.Key)
	case "replace":
		query.Set(rule.Key, rule.Value)
	default:
		return fmt.Errorf("不支持的query操作: %s", rule.Action)
	}

	req.URL.RawQuery = query.Encode()
	return nil
}

// processBody 处理请求体规则（流程编排）
func (h *RequestHandler) processBody(req *http.Request, rule Rule) error {
	// 1. 确保body已加载
	if err := h.ensureBodyLoaded(req); err != nil {
		return err
	}

	// 2. 判断是否为JSON操作
	if strings.HasPrefix(rule.Action, "json_") {
		return h.processJSONBody(req, rule)
	}

	// 3. 执行常规操作
	newBody, err := h.applyBodyOperation(h.body, rule)
	if err != nil {
		return err
	}

	// 4. 更新body和Content-Length
	h.updateBodyAndLength(req, newBody)
	return nil
}

// ensureBodyLoaded 确保请求体已加载
func (h *RequestHandler) ensureBodyLoaded(req *http.Request) error {
	if h.body == nil && req.Body != nil {
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			return fmt.Errorf("读取请求体失败: %w", err)
		}
		h.body = bodyBytes
		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	if h.body == nil {
		h.body = []byte{}
	}
	return nil
}

// applyBodyOperation 应用body操作
func (h *RequestHandler) applyBodyOperation(body []byte, rule Rule) ([]byte, error) {
	switch rule.Action {
	case "replace":
		return []byte(rule.Value), nil
	case "append", "add":
		return append(body, []byte(rule.Value)...), nil
	case "transform":
		return h.applyTransform(body, rule)
	case "remove":
		return []byte{}, nil
	default:
		return nil, fmt.Errorf("不支持的body操作: %s", rule.Action)
	}
}

// applyTransform 应用正则替换
func (h *RequestHandler) applyTransform(body []byte, rule Rule) ([]byte, error) {
	if rule.Pattern == "" {
		return nil, fmt.Errorf("body的transform操作需要提供pattern")
	}
	re, err := regexp.Compile(rule.Pattern)
	if err != nil {
		return nil, fmt.Errorf("正则表达式编译失败: %w", err)
	}
	return re.ReplaceAll(body, []byte(rule.Value)), nil
}

// updateBodyAndLength 更新body和内容长度
func (h *RequestHandler) updateBodyAndLength(req *http.Request, body []byte) {
	h.body = body
	req.ContentLength = int64(len(body))
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))
}

// processJSONBody 处理JSON专用的body规则
func (h *RequestHandler) processJSONBody(req *http.Request, rule Rule) error {
	// 解析当前body为JSON对象
	var jsonData map[string]interface{}
	if err := json.Unmarshal(h.body, &jsonData); err != nil {
		return fmt.Errorf("JSON解析失败: %w", err)
	}

	var err error
	// 根据action执行不同操作
	switch rule.Action {
	case "json_add":
		err = h.jsonAdd(jsonData, rule)
	case "json_remove":
		err = h.jsonRemove(jsonData, rule)
	case "json_replace":
		err = h.jsonReplace(jsonData, rule)
	case "json_merge":
		err = h.jsonMerge(jsonData, rule)
	default:
		return fmt.Errorf("不支持的JSON操作: %s", rule.Action)
	}

	if err != nil {
		return err
	}

	// 序列化回字节
	newBody, err := json.Marshal(jsonData)
	if err != nil {
		return fmt.Errorf("JSON序列化失败: %w", err)
	}

	// 更新body缓存
	h.body = newBody

	// 更新Content-Length头
	req.ContentLength = int64(len(newBody))
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(newBody)))

	return nil
}

// jsonAdd 添加JSON字段
func (h *RequestHandler) jsonAdd(data map[string]interface{}, rule Rule) error {
	value := parseJSONValue(rule.Value)

	if rule.JSONPath != "" {
		// 使用JSON路径
		return setJSONByPath(data, rule.JSONPath, value)
	}
	// 直接添加字段
	data[rule.Key] = value
	return nil
}

// jsonRemove 删除JSON字段
func (h *RequestHandler) jsonRemove(data map[string]interface{}, rule Rule) error {
	if rule.JSONPath != "" {
		return removeJSONByPath(data, rule.JSONPath)
	}
	delete(data, rule.Key)
	return nil
}

// jsonReplace 替换JSON字段值
func (h *RequestHandler) jsonReplace(data map[string]interface{}, rule Rule) error {
	value := parseJSONValue(rule.Value)

	if rule.JSONPath != "" {
		return setJSONByPath(data, rule.JSONPath, value)
	}
	if _, exists := data[rule.Key]; exists {
		data[rule.Key] = value
	}
	return nil
}

// jsonMerge 合并JSON对象
func (h *RequestHandler) jsonMerge(data map[string]interface{}, rule Rule) error {
	var mergeData map[string]interface{}
	if err := json.Unmarshal([]byte(rule.Value), &mergeData); err != nil {
		return fmt.Errorf("JSON合并数据解析失败: %w", err)
	}
	// 深度合并
	deepMerge(data, mergeData)
	return nil
}

// BuildRemoteURL 构建远程URL
func BuildRemoteURL(remoteURL string, req *http.Request) (string, error) {
	// 解析远程URL
	remote, err := url.Parse(remoteURL)
	if err != nil {
		return "", fmt.Errorf("解析远程URL失败: %w", err)
	}

	// 构建完整URL - 优先使用remoteURL中的路径
	fullURL := &url.URL{
		Scheme:   remote.Scheme,
		Host:     remote.Host,
		RawQuery: req.URL.RawQuery,
	}

	// 如果remoteURL包含路径，使用remoteURL的路径
	// 否则使用请求的路径
	if remote.Path != "" {
		fullURL.Path = remote.Path
	} else {
		fullURL.Path = req.URL.Path
	}

	return fullURL.String(), nil
}

// MatchRoute 根据请求路径匹配路由
func MatchRoute(routes []Route, path string) (*Route, error) {
	for i := range routes {
		route := &routes[i]

		// 使用预编译的正则表达式
		if route.compiledPattern == nil {
			return nil, fmt.Errorf("路由 %d: 正则表达式未编译", i)
		}

		// 匹配路径
		if route.compiledPattern.MatchString(path) {
			return route, nil
		}
	}

	return nil, fmt.Errorf("未找到匹配的路由: %s", path)
}

// parseJSONValue 智能解析JSON值
func parseJSONValue(value string) interface{} {
	// 尝试解析为JSON
	var parsed interface{}
	if err := json.Unmarshal([]byte(value), &parsed); err == nil {
		return parsed
	}
	// 返回字符串
	return value
}

// deepMerge 深度合并JSON对象
func deepMerge(dst, src map[string]interface{}) {
	for key, srcVal := range src {
		if dstVal, exists := dst[key]; exists {
			if dstMap, isMap := dstVal.(map[string]interface{}); isMap {
				if srcMap, isSrcMap := srcVal.(map[string]interface{}); isSrcMap {
					deepMerge(dstMap, srcMap)
					continue
				}
			}
		}
		dst[key] = srcVal
	}
}

// setJSONByPath 通过JSON路径设置值
func setJSONByPath(data map[string]interface{}, path string, value interface{}) error {
	// 移除开头的$
	path = strings.TrimPrefix(path, "$")

	// 解析路径
	parts := parseJSONPath(path)
	if len(parts) == 0 {
		return fmt.Errorf("无效的JSON路径: %s", path)
	}

	// 导航到父节点（创建模式）
	current, err := navigateJSONPathForSet(data, path, parts[:len(parts)-1])
	if err != nil {
		return err
	}

	// 设置最终值
	lastPart := parts[len(parts)-1]
	current[lastPart] = value

	return nil
}

// removeJSONByPath 通过JSON路径删除字段
func removeJSONByPath(data map[string]interface{}, path string) error {
	// 移除开头的$
	path = strings.TrimPrefix(path, "$")

	// 解析路径
	parts := parseJSONPath(path)
	if len(parts) == 0 {
		return fmt.Errorf("无效的JSON路径: %s", path)
	}

	// 导航到父节点（严格模式）
	current, err := navigateJSONPathForRemove(data, path, parts[:len(parts)-1])
	if err != nil {
		return err
	}

	// 删除字段
	lastPart := parts[len(parts)-1]
	delete(current, lastPart)

	return nil
}

// navigateJSONPathForSet 导航到JSON路径（设置模式：不存在则创建）
func navigateJSONPathForSet(data map[string]interface{}, originalPath string, parts []string) (map[string]interface{}, error) {
	current := data
	for i := 0; i < len(parts); i++ {
		part := parts[i]

		if strings.Contains(part, "[") {
			return nil, fmt.Errorf("暂不支持数组路径: %s", originalPath)
		}

		if next, exists := current[part]; exists {
			if nextMap, ok := next.(map[string]interface{}); ok {
				current = nextMap
			} else {
				return nil, fmt.Errorf("路径 %s 的中间节点不是对象", originalPath)
			}
		} else {
			// 创建新的对象
			newMap := make(map[string]interface{})
			current[part] = newMap
			current = newMap
		}
	}
	return current, nil
}

// navigateJSONPathForRemove 导航到JSON路径（删除模式：不存在则报错）
func navigateJSONPathForRemove(data map[string]interface{}, originalPath string, parts []string) (map[string]interface{}, error) {
	current := data
	for i := 0; i < len(parts); i++ {
		part := parts[i]

		if strings.Contains(part, "[") {
			return nil, fmt.Errorf("暂不支持数组路径: %s", originalPath)
		}

		if next, exists := current[part]; exists {
			if nextMap, ok := next.(map[string]interface{}); ok {
				current = nextMap
			} else {
				return nil, fmt.Errorf("路径 %s 的中间节点不是对象", originalPath)
			}
		} else {
			return nil, fmt.Errorf("路径 %s 不存在", originalPath)
		}
	}
	return current, nil
}

// parseJSONPath 解析JSON路径
func parseJSONPath(path string) []string {
	// 移除开头的.
	path = strings.TrimPrefix(path, ".")
	if path == "" {
		return []string{}
	}

	// 简单拆分路径
	parts := strings.Split(path, ".")

	// 处理数组索引（简化处理）
	var result []string
	for _, part := range parts {
		if part == "" {
			continue
		}
		result = append(result, part)
	}

	return result
}
