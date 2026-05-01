package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// ProxyConfig 代理配置
type ProxyConfig struct {
	Type     string `yaml:"type"`     // "http", "socks5", 或空表示不使用代理
	Address  string `yaml:"address"`  // 代理服务器地址
	Username string `yaml:"username"` // 可选：代理认证用户名
	Password string `yaml:"password"` // 可选：代理认证密码
}

// TokenConfig Token配置
type TokenConfig struct {
	Enabled bool     `yaml:"enabled"` // 是否启用Token轮换
	Tokens  []string `yaml:"tokens"`  // Token列表
	Header  string   `yaml:"header"`  // Token请求头名称，默认"Authorization"
	Prefix  string   `yaml:"prefix"`  // Token前缀，默认"Bearer"
}

// Route 路由配置
type Route struct {
	PathPattern string       `yaml:"path_pattern"`    // 路径匹配正则
	RemoteURL   string       `yaml:"remote_url"`      // 远程服务器URL
	Proxy       *ProxyConfig `yaml:"proxy,omitempty"` // 可选：路由代理配置
	Token       *TokenConfig `yaml:"token,omitempty"` // 可选：Token配置
	Rules       []Rule       `yaml:"rules"`           // 该路由的规则

	// 预编译的正则表达式（运行时使用，不序列化）
	compiledPattern *regexp.Regexp `yaml:"-"`
}

// Config 代理服务器配置
type Config struct {
	LocalPort int     `yaml:"local_port"`
	Timeout   int     `yaml:"timeout"`
	Debug     bool    `yaml:"debug"`  // 是否启用调试模式
	Routes    []Route `yaml:"routes"` // 路由配置列表
}

// Rule 请求处理规则
type Rule struct {
	Type     string `yaml:"type"`                // "header", "path", "query", "body"
	Action   string `yaml:"action"`              // "add", "remove", "replace", "append", "transform", "json_add", "json_remove", "json_replace", "json_merge"
	Key      string `yaml:"key"`                 // JSON字段名或JSON路径
	Value    string `yaml:"value"`               // JSON值或JSON对象
	Pattern  string `yaml:"pattern,omitempty"`   // 用于路径/body替换的正则表达式
	JSONPath string `yaml:"json_path,omitempty"` // JSON路径表达式
}

// LoadConfig 从YAML文件加载配置
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 设置默认值
	if config.LocalPort == 0 {
		config.LocalPort = 8080
	}
	if config.Timeout == 0 {
		config.Timeout = 30
	}

	// 验证配置
	if err := config.Validate(); err != nil {
		return nil, err
	}

	// 预编译所有路由的正则表达式
	if err := config.compileRoutePatterns(); err != nil {
		return nil, err
	}

	return &config, nil
}

// Validate 验证配置的有效性
func (c *Config) Validate() error {
	if c.LocalPort < 1 || c.LocalPort > 65535 {
		return fmt.Errorf("本地端口必须在 1-65535 范围内")
	}
	if c.Timeout < 1 {
		return fmt.Errorf("超时时间必须大于0")
	}
	if len(c.Routes) == 0 {
		return fmt.Errorf("至少需要配置一个路由")
	}

	// 验证路由
	hasDefaultRoute := false
	for i, route := range c.Routes {
		if err := validateRoute(&route, i); err != nil {
			return err
		}
		if route.PathPattern == ".*" {
			hasDefaultRoute = true
		}
	}

	// 默认路由现在是可选的，不再强制要求
	if !hasDefaultRoute {
		// 记录警告信息但不阻止启动
		fmt.Println("警告: 未配置默认路由（path_pattern: \".*\"），不匹配任何路由的请求将返回404")
	}

	return nil
}

// compileRoutePatterns 预编译所有路由的正则表达式
func (c *Config) compileRoutePatterns() error {
	for i := range c.Routes {
		re, err := regexp.Compile(c.Routes[i].PathPattern)
		if err != nil {
			return fmt.Errorf("路由 %d: 正则表达式编译失败: %w", i, err)
		}
		c.Routes[i].compiledPattern = re
	}
	return nil
}

// validateRoute 验证单个路由
func validateRoute(route *Route, index int) error {
	if route.PathPattern == "" {
		return fmt.Errorf("路由 %d: path_pattern不能为空", index)
	}
	if route.RemoteURL == "" {
		return fmt.Errorf("路由 %d: remote_url不能为空", index)
	}

	// 验证path_pattern是否为有效的正则表达式
	if _, err := regexp.Compile(route.PathPattern); err != nil {
		return fmt.Errorf("路由 %d: path_pattern正则表达式无效: %w", index, err)
	}

	// 验证代理配置
	if route.Proxy != nil {
		if err := validateProxyConfig(route.Proxy, index); err != nil {
			return err
		}
	}

	// 验证Token配置
	if route.Token != nil && route.Token.Enabled {
		if err := validateTokenConfig(route.Token, index); err != nil {
			return err
		}
	}

	// 验证规则
	for i, rule := range route.Rules {
		if err := validateRule(&rule, i); err != nil {
			return fmt.Errorf("路由 %d: %w", index, err)
		}
	}

	return nil
}

// validateProxyConfig 验证代理配置
func validateProxyConfig(proxy *ProxyConfig, routeIndex int) error {
	if proxy.Type == "" {
		return nil // 空类型表示不使用代理，合法
	}

	switch proxy.Type {
	case "http", "socks5":
		// 合法类型
	default:
		return fmt.Errorf("路由 %d: 不支持的代理类型 '%s'", routeIndex, proxy.Type)
	}

	if proxy.Address == "" {
		return fmt.Errorf("路由 %d: 代理地址不能为空", routeIndex)
	}

	return nil
}

// validateTokenConfig 验证Token配置
func validateTokenConfig(token *TokenConfig, routeIndex int) error {
	if len(token.Tokens) == 0 {
		return fmt.Errorf("路由 %d: 启用Token但tokens列表为空", routeIndex)
	}

	return nil
}

// validateRule 验证单个规则
func validateRule(rule *Rule, index int) error {
	switch rule.Type {
	case "header", "path", "query", "body":
		// 合法类型
	default:
		return fmt.Errorf("规则 %d: 不支持的规则类型 '%s'", index, rule.Type)
	}

	switch rule.Action {
	case "add", "remove", "replace", "append", "transform":
		// 通用操作
	case "json_add", "json_remove", "json_replace", "json_merge":
		// JSON专用操作
	default:
		return fmt.Errorf("规则 %d: 不支持的操作类型 '%s'", index, rule.Action)
	}

	// body类型的transform操作需要pattern
	if rule.Type == "body" && rule.Action == "transform" && rule.Pattern == "" {
		return fmt.Errorf("规则 %d: body的transform操作需要提供pattern", index)
	}

	// JSON操作的特殊验证
	if rule.Type == "body" && strings.HasPrefix(rule.Action, "json_") {
		if rule.Action == "json_merge" && rule.Value == "" {
			return fmt.Errorf("规则 %d: json_merge操作需要提供value", index)
		}
	}

	if rule.Action != "remove" && rule.Action != "json_remove" && rule.Value == "" {
		return fmt.Errorf("规则 %d: 非remove操作需要提供value", index)
	}

	return nil
}
