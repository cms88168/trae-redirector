package main

import (
	"log"
	"net/http"
	"strings"
	"sync"
)

// TokenManager Token管理器，处理Token轮换和状态
type TokenManager struct {
	mu           sync.Mutex
	tokens       []string
	currentIndex int
	header       string
	prefix       string
	origin       string // "use", "add", "ignore"
	failureCount int
}

// NewTokenManager 创建Token管理器
func NewTokenManager(config *TokenConfig) *TokenManager {
	header := config.Header
	if header == "" {
		header = "Authorization"
	}

	prefix := config.Prefix
	if prefix == "" {
		prefix = "Bearer"
	}

	origin := config.Origin
	if origin == "" {
		origin = "add"
	}

	return &TokenManager{
		tokens:       config.Tokens,
		currentIndex: 0,
		header:       header,
		prefix:       prefix,
		origin:       origin,
	}
}

// GetOrigin 获取origin策略
func (tm *TokenManager) GetOrigin() string {
	return tm.origin
}

// GetCurrentToken 获取当前池中Token
func (tm *TokenManager) GetCurrentToken() string {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.tokens[tm.currentIndex]
}

// GetCurrentIndex 获取当前Token索引
func (tm *TokenManager) GetCurrentIndex() int {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.currentIndex
}

// ApplyTokenToRequest 根据origin策略处理请求的Token
//
// 这是 TokenManager 对外的唯一入口，整合了收集和应用逻辑：
//   - "use": 透传 —— 不修改请求头，保留请求中原始Token
//   - "add": 收集+替换 —— 将请求中的Token收集到池，然后用池中当前Token替换请求头
//   - "ignore": 仅替换 —— 忽略请求中的Token，直接用池中当前Token替换请求头
func (tm *TokenManager) ApplyTokenToRequest(req *http.Request) {
	switch tm.origin {
	case "use":
		// 透传：不修改请求头
		return

	case "add":
		// 先收集请求中的Token到池
		tm.collectToken(req)
		// 再用池中Token替换请求头
		tm.setPoolToken(req)

	case "ignore":
		// 直接用池中Token替换请求头
		tm.setPoolToken(req)

	default:
		// 未知值按add处理
		tm.collectToken(req)
		tm.setPoolToken(req)
	}
}

// setPoolToken 将池中当前Token设置到请求头
func (tm *TokenManager) setPoolToken(req *http.Request) {
	token := tm.GetCurrentToken()
	var authValue string
	if tm.prefix != "" {
		authValue = tm.prefix + " " + token
	} else {
		authValue = token
	}
	req.Header.Set(tm.header, authValue)
}

// collectToken 从请求中提取Token，如果不在池中则添加
func (tm *TokenManager) collectToken(req *http.Request) {
	authHeader := req.Header.Get(tm.header)
	if authHeader == "" {
		return
	}

	// 提取纯token值（去掉prefix）
	tokenValue := authHeader
	if tm.prefix != "" && strings.HasPrefix(authHeader, tm.prefix+" ") {
		tokenValue = strings.TrimPrefix(authHeader, tm.prefix+" ")
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	for _, t := range tm.tokens {
		if t == tokenValue {
			return // 已存在
		}
	}

	log.Printf("[INFO] 收集新Token到池 (池大小: %d → %d)", len(tm.tokens), len(tm.tokens)+1)
	tm.tokens = append(tm.tokens, tokenValue)
}

// SwitchToNext 切换到下一个Token（认证失败时调用）
func (tm *TokenManager) SwitchToNext() string {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.currentIndex = (tm.currentIndex + 1) % len(tm.tokens)
	tm.failureCount = 0
	return tm.tokens[tm.currentIndex]
}

// RecordFailure 记录认证失败
func (tm *TokenManager) RecordFailure() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.failureCount++
}

// GetTokenCount 获取池中Token总数
func (tm *TokenManager) GetTokenCount() int {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return len(tm.tokens)
}
