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

	// 优雅关闭处理（捕获 Ctrl+C / 终止信号）
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("\n正在关闭代理服务器...")
		os.Exit(0)
	}()

	// 代理服务器放入后台 goroutine 启动，主线程交给托盘
	go func() {
		if err := proxy.Start(); err != nil {
			log.Fatalf("代理服务器启动失败: %v", err)
		}
	}()

	// 启动系统托盘（Windows 下显示右下角图标并自动隐藏控制台）
	// 非 Windows 平台为空实现，阻塞等待信号
	runTray(func() {
		log.Println("代理服务器已停止")
		os.Exit(0)
	})
}
