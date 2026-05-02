//go:build !windows
// +build !windows

package main

// 非 Windows 平台不启用系统托盘，保持原有前台运行行为
func runTray(onExitFn func()) {
	// 空阻塞，等待 SIGINT/SIGTERM 在 main.go 中处理
	select {}
}
