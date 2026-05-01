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

	return &TokenManager{
		tokens:       config.Tokens,
		currentIndex: 0,
		header:       header,
		prefix:       prefix,
	}
}

// GetCurrentToken 获取当前Token
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

// ApplyTokenToRequest 应用Token到请求
func (tm *TokenManager) ApplyTokenToRequest(req *http.Request) {
	token := tm.GetCurrentToken()
	var authValue string
	if tm.prefix != "" {
		authValue = tm.prefix + " " + token
	} else {
		authValue = token
	}
	req.Header.Set(tm.header, authValue)
}

// SwitchToNext 切换到下一个Token
func (tm *TokenManager) SwitchToNext() string {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.currentIndex = (tm.currentIndex + 1) % len(tm.tokens)
	tm.failureCount = 0

	return tm.tokens[tm.currentIndex]
}

// RecordFailure 记录失败
func (tm *TokenManager) RecordFailure() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.failureCount++
}

// GetFailureCount 获取失败次数
func (tm *TokenManager) GetFailureCount() int {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.failureCount
}

// GetTokenCount 获取Token总数（不需要锁，因为tokens切片在创建后不会改变）
func (tm *TokenManager) GetTokenCount() int {
	return len(tm.tokens)
}

// CheckAndAddToken 检查请求中的Token是否在池中，如果不在则添加
func (tm *TokenManager) CheckAndAddToken(req *http.Request) {
	// 从请求中提取Token
	tokenValue := tm.extractTokenFromRequest(req)
	if tokenValue == "" {
		return // 没有Token头，不处理
	}

	// 检查Token是否已在池中
	tm.mu.Lock()
	defer tm.mu.Unlock()

	for _, token := range tm.tokens {
		if token == tokenValue {
			// Token已在池中
			return
		}
	}

	// Token不在池中，添加到池
	log.Printf("[WARN] 发现新的Token，正在添加到Token池 (当前池大小: %d)", len(tm.tokens))
	tm.tokens = append(tm.tokens, tokenValue)
	log.Printf("[INFO] Token已添加到池，新池大小: %d", len(tm.tokens))
}

// extractTokenFromRequest 从请求中提取Token值（内部方法）
func (tm *TokenManager) extractTokenFromRequest(req *http.Request) string {
	authHeader := req.Header.Get(tm.header)
	if authHeader == "" {
		return ""
	}

	if tm.prefix != "" && strings.HasPrefix(authHeader, tm.prefix+" ") {
		return strings.TrimPrefix(authHeader, tm.prefix+" ")
	}

	return authHeader
}
