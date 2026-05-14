package main

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// ProxyConfig 代理配置
type ProxyConfig struct {
	Type     string `yaml:"type"`     // "http", "socks5", 或空表示不使用代理
	Address  string `yaml:"address"`  // 代理服务器地址
	Username string `yaml:"username"` // 可选：代理认证用户名
	Password string `yaml:"password"` // 可选：代理认证密码
}

// TokenConfig Token配置（存在即启用，无需enabled开关）
type TokenConfig struct {
	Origin string   `yaml:"origin"` // Token来源处理: "use"(只用请求原始Token), "add"(默认,收集请求Token到池并使用池Token), "ignore"(忽略请求Token,只用池Token)
	Tokens []string `yaml:"tokens"` // Token列表
	Header string   `yaml:"header"` // Token请求头名称，默认"Authorization"
	Prefix string   `yaml:"prefix"` // Token前缀，默认"Bearer"
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

// RouteEntry 路由条目（仅定义路径和config引用）
type RouteEntry struct {
	PathPattern string `yaml:"path_pattern"` // 路径匹配正则
	Config      string `yaml:"config"`       // 引用的config名称
}

// DetailConfig 详细配置（通过name标识）
type DetailConfig struct {
	Name      string       `yaml:"name"`            // 配置名称（唯一标识）
	RemoteURL string       `yaml:"remote_url"`      // 远程服务器URL
	Proxy     *ProxyConfig `yaml:"proxy,omitempty"` // 可选：代理配置
	Token     *TokenConfig `yaml:"token,omitempty"` // 可选：Token配置
	Rules     []Rule       `yaml:"rules"`           // 规则列表
}

// AppConfig 主配置文件结构（所有内容统一在一个config.yaml中）
type AppConfig struct {
	LocalPort int            `yaml:"local_port"` // 本地监听端口
	Timeout   int            `yaml:"timeout"`    // 请求超时时间（秒）
	Debug     bool           `yaml:"debug"`      // 调试模式
	Routes    []RouteEntry   `yaml:"routes"`     // 路由列表（路径 + config引用）
	Configs   []DetailConfig `yaml:"configs"`    // 详细配置列表（统一定义）
}

// Route 运行时路由（合并了路径和详细配置）
type Route struct {
	PathPattern string
	RemoteURL   string
	Proxy       *ProxyConfig
	Token       *TokenConfig
	Rules       []Rule

	// 预编译的正则表达式（运行时使用）
	compiledPattern *regexp.Regexp
}

// Config 代理服务器运行时配置（供Proxy使用）
type Config struct {
	LocalPort int
	Timeout   int
	Debug     bool
	Routes    []Route
}

// ConfigManager 配置管理器，支持动态切换config
type ConfigManager struct {
	mu            sync.RWMutex
	appConfig     *AppConfig
	detailConfigs map[string]*DetailConfig // 按name索引的configs快速查找表
	configPath    string                   // 配置文件路径
}

// NewConfigManager 创建配置管理器
func NewConfigManager(configPath string) *ConfigManager {
	return &ConfigManager{
		configPath:    configPath,
		detailConfigs: make(map[string]*DetailConfig),
	}
}

// Load 加载配置文件
func (cm *ConfigManager) Load() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// 读取配置文件
	data, err := os.ReadFile(cm.configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}

	var appConfig AppConfig
	if err := yaml.Unmarshal(data, &appConfig); err != nil {
		return fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 设置默认值
	if appConfig.LocalPort == 0 {
		appConfig.LocalPort = 8080
	}
	if appConfig.Timeout == 0 {
		appConfig.Timeout = 30
	}

	cm.appConfig = &appConfig

	// 构建configs索引
	cm.detailConfigs = make(map[string]*DetailConfig)
	for i := range appConfig.Configs {
		cfg := &appConfig.Configs[i]
		if cfg.Name == "" {
			return fmt.Errorf("configs[%d]: name不能为空", i)
		}
		if _, exists := cm.detailConfigs[cfg.Name]; exists {
			return fmt.Errorf("configs中存在重复的name: '%s'", cfg.Name)
		}
		cm.detailConfigs[cfg.Name] = cfg
	}

	// 验证所有路由引用的config都存在
	for i, route := range cm.appConfig.Routes {
		if route.Config == "" {
			return fmt.Errorf("路由 %d (%s): config不能为空", i, route.PathPattern)
		}
		if _, exists := cm.detailConfigs[route.Config]; !exists {
			return fmt.Errorf("路由 %d (%s): 引用的config '%s' 不存在", i, route.PathPattern, route.Config)
		}
	}

	return nil
}

// ResolveConfig 解析出运行时Config（合并路由和detail config）
func (cm *ConfigManager) ResolveConfig() (*Config, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	config := &Config{
		LocalPort: cm.appConfig.LocalPort,
		Timeout:   cm.appConfig.Timeout,
		Debug:     cm.appConfig.Debug,
	}

	for i, entry := range cm.appConfig.Routes {
		detail, exists := cm.detailConfigs[entry.Config]
		if !exists {
			return nil, fmt.Errorf("路由 %d: config '%s' 不存在", i, entry.Config)
		}

		route := Route{
			PathPattern: entry.PathPattern,
			RemoteURL:   detail.RemoteURL,
			Proxy:       detail.Proxy,
			Token:       detail.Token,
			Rules:       detail.Rules,
		}

		// 编译正则
		re, err := regexp.Compile(route.PathPattern)
		if err != nil {
			return nil, fmt.Errorf("路由 %d: 正则表达式编译失败: %w", i, err)
		}
		route.compiledPattern = re

		config.Routes = append(config.Routes, route)
	}

	// 验证
	if err := validateConfig(config); err != nil {
		return nil, err
	}

	return config, nil
}

// SwitchRouteConfig 切换某个路由使用的config
func (cm *ConfigManager) SwitchRouteConfig(routeIndex int, newConfigName string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if routeIndex < 0 || routeIndex >= len(cm.appConfig.Routes) {
		return fmt.Errorf("无效的路由索引: %d", routeIndex)
	}

	if _, exists := cm.detailConfigs[newConfigName]; !exists {
		return fmt.Errorf("config '%s' 不存在", newConfigName)
	}

	cm.appConfig.Routes[routeIndex].Config = newConfigName
	return nil
}

// GetRoutes 获取当前路由列表
func (cm *ConfigManager) GetRoutes() []RouteEntry {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	routes := make([]RouteEntry, len(cm.appConfig.Routes))
	copy(routes, cm.appConfig.Routes)
	return routes
}

// GetAllConfigNames 获取所有可用的config名称
func (cm *ConfigManager) GetAllConfigNames() []string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	names := make([]string, 0, len(cm.detailConfigs))
	for name := range cm.detailConfigs {
		names = append(names, name)
	}
	return names
}

// GetAppConfig 获取主配置（只读）
func (cm *ConfigManager) GetAppConfig() *AppConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.appConfig
}

// SaveMainConfig 保存当前配置到文件（用于持久化切换结果）
func (cm *ConfigManager) SaveMainConfig() error {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	data, err := yaml.Marshal(cm.appConfig)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	if err := os.WriteFile(cm.configPath, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	return nil
}

// validateConfig 验证运行时配置
func validateConfig(c *Config) error {
	if c.LocalPort < 1 || c.LocalPort > 65535 {
		return fmt.Errorf("本地端口必须在 1-65535 范围内")
	}
	if c.Timeout < 1 {
		return fmt.Errorf("超时时间必须大于0")
	}
	if len(c.Routes) == 0 {
		return fmt.Errorf("至少需要配置一个路由")
	}

	hasDefaultRoute := false
	for i, route := range c.Routes {
		if err := validateResolvedRoute(&route, i); err != nil {
			return err
		}
		if route.PathPattern == ".*" {
			hasDefaultRoute = true
		}
	}

	if !hasDefaultRoute {
		log.Println("警告: 未配置默认路由（path_pattern: \".*\"），不匹配任何路由的请求将返回404")
	}

	return nil
}

// validateResolvedRoute 验证解析后的路由
func validateResolvedRoute(route *Route, index int) error {
	if route.PathPattern == "" {
		return fmt.Errorf("路由 %d: path_pattern不能为空", index)
	}
	if route.RemoteURL == "" {
		return fmt.Errorf("路由 %d: remote_url不能为空", index)
	}

	if _, err := regexp.Compile(route.PathPattern); err != nil {
		return fmt.Errorf("路由 %d: path_pattern正则表达式无效: %w", index, err)
	}

	// 验证代理配置
	if route.Proxy != nil {
		if err := validateProxyConfig(route.Proxy, index); err != nil {
			return err
		}
	}

	// 验证Token配置（token字段存在且有tokens即启用）
	if route.Token != nil && len(route.Token.Tokens) > 0 {
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
		return nil
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

	// 验证origin字段
	switch token.Origin {
	case "", "use", "add", "ignore":
		// 合法值（空默认为add）
	default:
		return fmt.Errorf("路由 %d: 不支持的token origin '%s'（可选: use, add, ignore）", routeIndex, token.Origin)
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
