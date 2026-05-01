package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Proxy 代理服务器
type Proxy struct {
	config        *Config
	tokenMu       sync.RWMutex
	tokenManagers map[string]*TokenManager
}

// NewProxy 创建代理服务器实例
func NewProxy(config *Config) *Proxy {
	return &Proxy{
		config:        config,
		tokenManagers: make(map[string]*TokenManager),
	}
}

// createHTTPClient 创建HTTP客户端，支持代理配置
// 注意：不设置 http.Client.Timeout，避免对流式响应（SSE/chunked）造成整体超时；
// 仅通过 Transport.ResponseHeaderTimeout 守护“拿不到响应头”的场景。
func createHTTPClient(timeout int, proxyConfig *ProxyConfig) (*http.Client, error) {
	transport := &http.Transport{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: time.Duration(timeout) * time.Second,
	}

	// 配置代理
	if proxyConfig != nil && proxyConfig.Type != "" && proxyConfig.Address != "" {
		proxyURL, err := url.Parse(proxyConfig.Address)
		if err != nil {
			return nil, fmt.Errorf("解析代理地址失败: %w", err)
		}

		// 先设置认证信息，再设置代理
		if proxyConfig.Username != "" {
			proxyURL.User = url.UserPassword(proxyConfig.Username, proxyConfig.Password)
		}

		transport.Proxy = http.ProxyURL(proxyURL)
		log.Printf("使用代理: %s (%s)", proxyConfig.Type, proxyConfig.Address)
	}

	return &http.Client{
		Transport: transport,
	}, nil
}

// Start 启动代理服务器
func (p *Proxy) Start() error {
	addr := fmt.Sprintf(":%d", p.config.LocalPort)

	log.Printf("代理服务器启动在 http://localhost%s", addr)
	log.Printf("配置路由数: %d", len(p.config.Routes))
	for i, route := range p.config.Routes {
		tokenInfo := ""
		if route.Token != nil && route.Token.Enabled {
			tokenInfo = fmt.Sprintf(" (%d个Token)", len(route.Token.Tokens))
		}
		log.Printf("路由 %d: %s -> %s (%d条规则)%s", i+1, route.PathPattern, route.RemoteURL, len(route.Rules), tokenInfo)
	}
	log.Printf("超时设置: %d秒", p.config.Timeout)

	// 创建HTTP服务器
	// 注意：为适配流式响应（SSE/chunked），不设置 ReadTimeout/WriteTimeout；
	// 仅通过 ReadHeaderTimeout 限制请求头读取时间。
	server := &http.Server{
		Addr:              addr,
		Handler:           http.HandlerFunc(p.serveHTTP),
		ReadHeaderTimeout: 30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// 启动服务器
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("启动HTTP服务器失败: %w", err)
	}

	return nil
}

// getTokenManager 获取路由的Token管理器
func (p *Proxy) getTokenManager(route *Route) *TokenManager {
	if route.Token == nil || !route.Token.Enabled {
		return nil
	}

	// 从缓存获取或创建
	p.tokenMu.Lock()
	defer p.tokenMu.Unlock()

	key := route.PathPattern
	if manager, exists := p.tokenManagers[key]; exists {
		return manager
	}

	manager := NewTokenManager(route.Token)
	p.tokenManagers[key] = manager
	return manager
}

// cloneRequest 克隆请求（用于重试）
func cloneRequest(r *http.Request, bodyBytes []byte) *http.Request {
	newReq := r.Clone(r.Context())
	if len(bodyBytes) > 0 {
		newReq.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}
	return newReq
}

// serveHTTP 处理HTTP请求（流程编排）
func (p *Proxy) serveHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("收到请求: %s %s", r.Method, r.URL.String())

	// Debug模式：打印请求详细信息
	if p.config.Debug {
		p.logRequestDetails(r)
	}

	// 1. 匹配路由
	matchedRoute, err := p.matchRoute(r.URL.Path)
	if err != nil {
		p.handleRouteNotFoundError(w, err)
		return
	}

	log.Printf("匹配路由: %s -> %s", matchedRoute.PathPattern, matchedRoute.RemoteURL)

	// 2. 读取请求体
	bodyBytes, err := p.readRequestBody(r)
	if err != nil {
		p.handleReadBodyError(w, err)
		return
	}

	// Debug模式：打印请求体
	if p.config.Debug && len(bodyBytes) > 0 {
		log.Printf("[DEBUG] 请求体:\n%s", string(bodyBytes))
	}

	// 3. 执行请求（带重试）
	resp, err := p.executeWithRetry(r, matchedRoute, bodyBytes)
	if err != nil {
		p.handleRequestError(w, err)
		return
	}
	defer resp.Body.Close()

	// 4. 返回响应
	p.forwardResponse(w, r, resp)
}

// logRequestDetails 打印请求的详细信息（Debug模式）
func (p *Proxy) logRequestDetails(r *http.Request) {
	log.Printf("[DEBUG] ===== 请求详细信息 =====")
	log.Printf("[DEBUG] 方法: %s", r.Method)
	log.Printf("[DEBUG] URL: %s", r.URL.String())
	log.Printf("[DEBUG] 协议: %s", r.Proto)
	log.Printf("[DEBUG] Host: %s", r.Host)
	log.Printf("[DEBUG] 内容长度: %d", r.ContentLength)

	// 打印请求头
	log.Printf("[DEBUG] 请求头:")
	for key, values := range r.Header {
		for _, value := range values {
			log.Printf("[DEBUG]   %s: %s", key, value)
		}
	}

	// 打印查询参数
	if len(r.URL.Query()) > 0 {
		log.Printf("[DEBUG] 查询参数:")
		for key, values := range r.URL.Query() {
			for _, value := range values {
				log.Printf("[DEBUG]   %s: %s", key, value)
			}
		}
	}

	log.Printf("[DEBUG] ============================")
}

// matchRoute 匹配路由
func (p *Proxy) matchRoute(path string) (*Route, error) {
	return MatchRoute(p.config.Routes, path)
}

// readRequestBody 读取请求体
func (p *Proxy) readRequestBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	return io.ReadAll(r.Body)
}

// executeWithRetry 执行请求（带Token重试）
func (p *Proxy) executeWithRetry(r *http.Request, route *Route, bodyBytes []byte) (*http.Response, error) {
	tokenManager := p.getTokenManager(route)
	maxRetries := p.getRetryCount(tokenManager)

	if tokenManager != nil {
		log.Printf("Token轮换已启用，Token数量: %d", maxRetries)
	}

	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if tokenManager != nil {
			log.Printf("使用Token索引: %d", tokenManager.GetCurrentIndex())
		}

		resp, err := p.attemptRequest(r, route, bodyBytes, tokenManager, attempt)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// 如枟是认证失败且有Token管理器，继续重试
		if !p.isAuthError(err) || tokenManager == nil {
			break
		}

		tokenManager.SwitchToNext()
		log.Printf("已切换到Token索引: %d", tokenManager.GetCurrentIndex())
	}

	return nil, lastErr
}

// attemptRequest 尝试单次请求
func (p *Proxy) attemptRequest(r *http.Request, route *Route, bodyBytes []byte,
	tokenManager *TokenManager, attempt int) (*http.Response, error) {

	// 1. 克隆请求
	retryReq := cloneRequest(r, bodyBytes)

	// 2. 应用规则
	handler := NewRequestHandler(route.Rules)
	if len(bodyBytes) > 0 {
		handler.SetBody(bodyBytes)
	}
	if err := handler.ProcessRequest(retryReq); err != nil {
		return nil, fmt.Errorf("处理请求失败: %w", err)
	}

	// 获取处理后的请求体
	modifiedBody := handler.GetBody()

	// 2.5 如果启用了Token管理，检查并添加请求中的新Token
	if tokenManager != nil {
		tokenManager.CheckAndAddToken(retryReq)
	}

	// 3. 构建远程URL
	remoteURL, err := BuildRemoteURL(route.RemoteURL, retryReq)
	if err != nil {
		return nil, fmt.Errorf("构建远程URL失败: %w", err)
	}

	// 4. 创建客户端
	client, err := createHTTPClient(p.config.Timeout, route.Proxy)
	if err != nil {
		return nil, fmt.Errorf("创建HTTP客户端失败: %w", err)
	}

	// 5. 创建远程请求
	remoteReq, err := p.createRemoteRequest(r, retryReq, remoteURL, tokenManager, handler)
	if err != nil {
		return nil, err
	}

	// Debug模式：打印处理后的远程请求信息
	if p.config.Debug {
		log.Printf("[DEBUG] ===== 处理后远程请求 =====")
		log.Printf("[DEBUG] 远程URL: %s", remoteURL)
		log.Printf("[DEBUG] 方法: %s", remoteReq.Method)
		log.Printf("[DEBUG] 远程请求头:")
		for key, values := range remoteReq.Header {
			for _, value := range values {
				log.Printf("[DEBUG]   %s: %s", key, value)
			}
		}
		if len(modifiedBody) > 0 {
			log.Printf("[DEBUG] 处理后请求体:\n%s", string(modifiedBody))
		}
		log.Printf("[DEBUG] ================================")
	}

	log.Printf("转发到: %s (尝试 %d/%d)", remoteURL, attempt+1, p.getRetryCount(tokenManager))

	// 6. 发送请求
	resp, err := client.Do(remoteReq)
	if err != nil {
		return nil, fmt.Errorf("远程请求失败: %w", err)
	}

	// 7. 检查认证失败
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		resp.Body.Close()
		if tokenManager != nil {
			tokenManager.RecordFailure()
		}
		return nil, fmt.Errorf("认证失败 (状态码: %d)", resp.StatusCode)
	}

	return resp, nil
}

// createRemoteRequest 创建远程请求
func (p *Proxy) createRemoteRequest(r *http.Request, retryReq *http.Request,
	remoteURL string, tokenManager *TokenManager, handler *RequestHandler) (*http.Request, error) {

	var bodyReader io.Reader
	modifiedBody := handler.GetBody()
	if len(modifiedBody) > 0 {
		bodyReader = bytes.NewBuffer(modifiedBody)
	}

	remoteReq, err := http.NewRequestWithContext(r.Context(), r.Method, remoteURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("创建远程请求失败: %w", err)
	}

	remoteReq.Header = retryReq.Header.Clone()
	if host := remoteReq.Header.Get("Host"); host != "" {
		remoteReq.Host = host
	}

	if tokenManager != nil {
		tokenManager.ApplyTokenToRequest(remoteReq)
	}

	return remoteReq, nil
}

// 错误处理函数
func (p *Proxy) handleRouteNotFoundError(w http.ResponseWriter, err error) {
	log.Printf("路由匹配失败: %v", err)
	http.Error(w, "路由匹配失败", http.StatusNotFound)
}

func (p *Proxy) handleReadBodyError(w http.ResponseWriter, err error) {
	log.Printf("读取请求体失败: %v", err)
	http.Error(w, "读取请求体失败", http.StatusInternalServerError)
}

func (p *Proxy) handleRequestError(w http.ResponseWriter, err error) {
	log.Printf("远程请求失败: %v", err)
	http.Error(w, "远程请求失败", http.StatusBadGateway)
}

// forwardResponse 转发响应
func (p *Proxy) forwardResponse(w http.ResponseWriter, r *http.Request, resp *http.Response) {
	defer resp.Body.Close()
	log.Printf("收到响应: %d %s", resp.StatusCode, resp.Status)

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	streaming := isStreamingResponse(resp)
	if streaming {
		// 流式响应使用 chunked 编码，删除 Content-Length 避免冲突
		w.Header().Del("Content-Length")
	}

	if p.config.Debug {
		log.Printf("[DEBUG] ===== 响应头 =====")
		for key, values := range resp.Header {
			for _, value := range values {
				log.Printf("[DEBUG]   %s: %s", key, value)
			}
		}
		if streaming {
			log.Printf("[DEBUG] 流式响应开始转发（按块 flush）")
		}
		log.Printf("[DEBUG] ==================")
	}

	w.WriteHeader(resp.StatusCode)

	if streaming {
		p.streamCopy(w, resp.Body, r)
	} else if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("写入响应失败: %v", err)
	}

	log.Printf("响应完成: %s %s", r.Method, r.URL.String())
}

// isStreamingResponse 判断响应是否为流式响应
func isStreamingResponse(resp *http.Response) bool {
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(ct, "text/event-stream") ||
		strings.Contains(ct, "application/x-ndjson") ||
		strings.Contains(ct, "application/stream+json") {
		return true
	}
	for _, te := range resp.TransferEncoding {
		if strings.EqualFold(te, "chunked") {
			return true
		}
	}
	return false
}

// streamCopy 按块转发并 flush，适配 SSE/chunked 流式响应
func (p *Proxy) streamCopy(w http.ResponseWriter, body io.Reader, r *http.Request) {
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 4096)
	for {
		n, err := body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				log.Printf("写入流式响应失败: %v", werr)
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			if isStreamEndError(err, r) {
				log.Printf("流式转发结束: %v", err)
			} else {
				log.Printf("读取上游流失败: %v", err)
			}
			return
		}
		if r.Context().Err() != nil {
			log.Printf("客户端已断开，结束流式转发")
			return
		}
	}
}

// isStreamEndError 判断是否为流式正常结束（EOF 或客户端断开导致的 context 取消）
func isStreamEndError(err error, r *http.Request) bool {
	if err == io.EOF {
		return true
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// 请求上下文已结束，视为客户端断开导致的正常收尾
	if r.Context().Err() != nil {
		return true
	}
	return false
}

// getRetryCount 获取重试次数
func (p *Proxy) getRetryCount(tokenManager *TokenManager) int {
	if tokenManager == nil {
		return 1
	}
	return tokenManager.GetTokenCount()
}

// isAuthError 判断是否为认证错误
func (p *Proxy) isAuthError(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "认证失败") ||
		strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "403"))
}
