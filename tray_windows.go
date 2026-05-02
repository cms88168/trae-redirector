//go:build windows
// +build windows

package main

import (
	"log"
	"sync/atomic"
	"syscall"

	"github.com/getlantern/systray"
)

// Windows API 动态加载
var (
	modKernel32           = syscall.NewLazyDLL("kernel32.dll")
	modUser32             = syscall.NewLazyDLL("user32.dll")
	procGetConsoleWindow  = modKernel32.NewProc("GetConsoleWindow")
	procShowWindow        = modUser32.NewProc("ShowWindow")
	procSetForegroundWind = modUser32.NewProc("SetForegroundWindow")
)

const (
	swHide = 0
	swShow = 5
)

// consoleVisible 记录当前控制台窗口是否可见（原子访问）
var consoleVisible atomic.Bool

// setConsoleVisible 显示或隐藏当前进程关联的控制台窗口
// 当程序以 GUI 模式(-H=windowsgui)编译时 GetConsoleWindow 返回 0，自动忽略
func setConsoleVisible(visible bool) {
	hwnd, _, _ := procGetConsoleWindow.Call()
	if hwnd == 0 {
		return
	}
	cmd := uintptr(swHide)
	if visible {
		cmd = swShow
	}
	procShowWindow.Call(hwnd, cmd)
	if visible {
		procSetForegroundWind.Call(hwnd)
	}
	consoleVisible.Store(visible)
}

// runTray 在主 goroutine 中启动系统托盘，阻塞直到退出
// onReadyFn 在托盘创建就绪时被调用
// onExitFn 在托盘退出前被调用（用于资源清理）
func runTray(onExitFn func()) {
	systray.Run(onTrayReady, func() {
		if onExitFn != nil {
			onExitFn()
		}
	})
}

func onTrayReady() {
	systray.SetIcon(defaultTrayIcon)
	systray.SetTitle("")
	systray.SetTooltip("Trae Redirector - HTTP 代理")

	mShow := systray.AddMenuItem("显示控制台", "显示日志控制台窗口")
	mHide := systray.AddMenuItem("隐藏控制台", "隐藏日志控制台窗口")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("退出", "退出 Trae Redirector")

	// 启动后自动隐藏控制台，仅保留托盘图标
	setConsoleVisible(false)

	go func() {
		for {
			select {
			case <-mShow.ClickedCh:
				setConsoleVisible(true)
			case <-mHide.ClickedCh:
				setConsoleVisible(false)
			case <-mQuit.ClickedCh:
				log.Println("用户通过托盘菜单请求退出")
				systray.Quit()
				return
			}
		}
	}()
}
