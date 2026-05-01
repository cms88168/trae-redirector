package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// 默认配置文件路径
	configPath := "config.yaml"

	// 检查配置文件是否存在
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Fatalf("配置文件不存在: %s", configPath)
	}

	// 加载配置
	log.Printf("正在加载配置文件: %s", configPath)
	config, err := LoadConfig(configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	log.Printf("配置加载成功")
	log.Printf("本地端口: %d", config.LocalPort)
	log.Printf("路由数量: %d", len(config.Routes))
	log.Printf("超时时间: %d秒", config.Timeout)
	log.Printf("调试模式: %v", config.Debug)

	// 创建代理
	proxy := NewProxy(config)

	// 优雅关闭处理
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("\n正在关闭代理服务器...")
		os.Exit(0)
	}()

	// 启动代理服务器
	if err := proxy.Start(); err != nil {
		log.Fatalf("代理服务器启动失败: %v", err)
	}
}
