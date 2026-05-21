//go:build windows
// +build windows

package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"unsafe"

	"github.com/getlantern/systray"
)

// Windows API
var (
	modKernel32       = syscall.NewLazyDLL("kernel32.dll")
	procAttachConsole = modKernel32.NewProc("AttachConsole")
	procCreateFileW   = modKernel32.NewProc("CreateFileW")
	procSetStdHandle  = modKernel32.NewProc("SetStdHandle")
	modUser32         = syscall.NewLazyDLL("user32.dll")
	procFindWindowW   = modUser32.NewProc("FindWindowW")
	procSendMessageW  = modUser32.NewProc("SendMessageW")
)

const (
	attachParentProcess uint32 = ^uint32(0) // ATTACH_PARENT_PROCESS = (DWORD)-1
	WM_CLOSE                   = 0x0010     // Windows消息：关闭窗口
)

// stdOutputHandle/stdErrorHandle 用补码表示负数常量
var (
	stdOutputHandle = ^uintptr(10) // STD_OUTPUT_HANDLE = (DWORD)-11
	stdErrorHandle  = ^uintptr(11) // STD_ERROR_HANDLE  = (DWORD)-12
)

// logFilePath 日志文件完整路径
var logFilePath string

// logFile 日志文件句柄（全局保持打开）
var logFile *os.File

// logMu 保护日志文件
var logMu sync.Mutex

// AttachParentConsole 初始化日志输出
//
// 程序必须以 GUI 子系统编译（go build -ldflags "-H windowsgui"），
// Windows 不会为 GUI 程序创建控制台，也就不会拉起 Windows Terminal。
//
// 本函数执行两件事：
//  1. 始终创建日志文件（用于持久化和托盘"打开日志"功能）
//  2. 尝试 AttachConsole(ATTACH_PARENT_PROCESS) 附加到父终端：
//     - 成功（从 cmd/PowerShell 启动）：同时输出到控制台和日志文件
//     - 失败（双击启动）：仅输出到日志文件
//
// 必须在 main() 中第一行调用，在任何 log.Print 之前。
func AttachParentConsole() {
	// 1. 设置日志文件
	initLogFile()

	// 2. 尝试附加到父进程控制台
	r, _, _ := procAttachConsole.Call(uintptr(attachParentProcess))
	if r != 0 {
		// 成功附加 → 重定向 stdout/stderr 到控制台
		consoleFd := openConsoleOutput()
		if consoleFd != nil && logFile != nil {
			// MultiWriter: 同时输出到控制台和日志文件
			mw := io.MultiWriter(consoleFd, logFile)
			log.SetOutput(mw)
			os.Stdout = consoleFd
			os.Stderr = consoleFd
		} else if consoleFd != nil {
			log.SetOutput(consoleFd)
			os.Stdout = consoleFd
			os.Stderr = consoleFd
		}
	} else {
		// AttachConsole 失败
		// 检测是否已经有控制台（非 GUI 模式编译）
		consoleFd := openConsoleOutput()
		if consoleFd != nil {
			// 已经有控制台输出（控制台模式编译）→ 同时输出到控制台和日志文件
			if logFile != nil {
				mw := io.MultiWriter(consoleFd, logFile)
				log.SetOutput(mw)
				os.Stdout = consoleFd
				os.Stderr = consoleFd
			} else {
				log.SetOutput(consoleFd)
				os.Stdout = consoleFd
				os.Stderr = consoleFd
			}
		} else {
			// 确实没有控制台（GUI模式，双击启动）→ 仅文件输出
			if logFile != nil {
				log.SetOutput(logFile)
			}
		}
	}
}

// initLogFile 创建/打开日志文件
func initLogFile() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	// 日志文件放在可执行文件同目录下
	exePath, err := os.Executable()
	if err != nil {
		exePath = "."
	}
	logFilePath = filepath.Join(filepath.Dir(exePath), "trae-redirector.log")

	f, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		// 回退到工作目录
		logFilePath = "trae-redirector.log"
		f, err = os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return
		}
	}
	logFile = f
}

// openConsoleOutput 打开已附加控制台的输出句柄
func openConsoleOutput() *os.File {
	conout, _ := syscall.UTF16PtrFromString("CONOUT$")
	handle, _, _ := procCreateFileW.Call(
		uintptr(unsafe.Pointer(conout)),
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		syscall.FILE_SHARE_WRITE,
		0,
		syscall.OPEN_EXISTING,
		0,
		0,
	)
	if handle == uintptr(syscall.InvalidHandle) {
		return nil
	}

	procSetStdHandle.Call(stdOutputHandle, handle)
	procSetStdHandle.Call(stdErrorHandle, handle)

	return os.NewFile(handle, "CONOUT$")
}

// openLogFile 使用类似tail的命令实时查看日志文件
func openLogFile() {
	logMu.Lock()
	defer logMu.Unlock()

	if logFilePath == "" {
		return
	}

	// 检查日志文件是否存在
	if _, err := os.Stat(logFilePath); os.IsNotExist(err) {
		log.Printf("日志文件不存在: %s", logFilePath)
		return
	}

	// 如果已有日志窗口，不再打开
	if isLogViewerOpen() {
		log.Printf("日志监控窗口已打开")
		return
	}

	// 构建PowerShell内联命令
	// 使用Get-Content -Wait -Tail实时监控日志，UTF-8编码读取
	powershellCmd := fmt.Sprintf(
		"$host.UI.RawUI.WindowTitle='Trae Redirector 日志监控'; "+
			"[Console]::OutputEncoding=[System.Text.Encoding]::UTF8; "+
			"$OutputEncoding=[System.Text.Encoding]::UTF8; "+
			"$sep='=' * 40; "+
			"Write-Host $sep -ForegroundColor Cyan; "+
			"Write-Host '  Trae Redirector 日志监控' -ForegroundColor Cyan; "+
			"Write-Host $sep -ForegroundColor Cyan; "+
			"Write-Host ''; "+
			"Write-Host '日志文件: ' -NoNewline -ForegroundColor Green; "+
			"Write-Host '%s' -ForegroundColor White; "+
			"Write-Host '显示最后50行，实时监控新日志' -ForegroundColor Yellow; "+
			"Write-Host '按 Ctrl+C 停止监控' -ForegroundColor Yellow; "+
			"Write-Host $sep -ForegroundColor Cyan; "+
			"Write-Host ''; "+
			"Get-Content -Path '%s' -Wait -Tail 50 -Encoding UTF8",
		logFilePath, logFilePath,
	)

	cmdLine := fmt.Sprintf(`/C start "Trae Redirector 日志监控" powershell -NoExit -Command "%s"`, powershellCmd)

	cmd := exec.Command("cmd")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: cmdLine,
	}

	if err := cmd.Start(); err != nil {
		log.Printf("打开日志文件失败: %v", err)
	}
}

// findWindowByTitle 通过窗口标题查找窗口句柄
func findWindowByTitle(title string) uintptr {
	windowTitle, _ := syscall.UTF16PtrFromString(title)

	// 只使用窗口标题查找，不指定类名（第一个参数为0）
	hwnd, _, _ := procFindWindowW.Call(
		0,
		uintptr(unsafe.Pointer(windowTitle)),
	)

	return hwnd
}

// closeLogViewer 关闭日志查看器窗口
// 返回是否找到了窗口并关闭
func closeLogViewer() bool {
	hwnd := findWindowByTitle("Trae Redirector 日志监控")

	if hwnd != 0 {
		log.Printf("正在关闭日志监控窗口...")
		// 发送WM_CLOSE消息关闭窗口
		procSendMessageW.Call(
			hwnd,
			WM_CLOSE,
			0,
			0,
		)
		return true
	}
	return false
}

// isLogViewerOpen 检查日志查看器窗口是否已打开
func isLogViewerOpen() bool {
	hwnd := findWindowByTitle("Trae Redirector 日志监控")
	return hwnd != 0
}

// --- 系统托盘 ---

var (
	trayProxy         *Proxy
	trayConfigManager *ConfigManager
	// 保存子菜单项引用，以便更新打勾状态
	trayRouteMenus  []*systray.MenuItem
	trayConfigMenus [][]*systray.MenuItem
)

// runTray 在主 goroutine 中启动系统托盘，阻塞直到退出
func runTray(proxy *Proxy, configManager *ConfigManager, onExitFn func()) {
	trayProxy = proxy
	trayConfigManager = configManager

	systray.Run(onTrayReady, func() {
		// 关闭日志监控窗口（如果已打开）
		if closeLogViewer() {
			log.Printf("日志监控窗口已关闭")
		}

		if onExitFn != nil {
			onExitFn()
		}
	})
}

func onTrayReady() {
	systray.SetIcon(defaultTrayIcon)
	systray.SetTitle("")
	systray.SetTooltip("Trae Redirector - HTTP 代理")

	mOpenLog := systray.AddMenuItem("打开日志", "使用tail模式实时查看日志文件")
	systray.AddSeparator()

	// 添加路由配置切换菜单
	buildRouteConfigMenu()

	systray.AddSeparator()
	mQuit := systray.AddMenuItem("退出", "退出 Trae Redirector")

	go func() {
		for {
			select {
			case <-mOpenLog.ClickedCh:
				openLogFile()
			case <-mQuit.ClickedCh:
				log.Println("用户通过托盘菜单请求退出")
				systray.Quit()
				return
			}
		}
	}()
}

// buildRouteConfigMenu 构建路由配置切换菜单
func buildRouteConfigMenu() {
	if trayConfigManager == nil {
		return
	}

	routes := trayConfigManager.GetRoutes()
	allConfigs := trayConfigManager.GetAllConfigNames()
	sort.Strings(allConfigs)

	// 清空之前的菜单引用
	trayRouteMenus = nil
	trayConfigMenus = nil

	for i, route := range routes {
		title := fmt.Sprintf("[%s] → %s", route.PathPattern, route.Config)
		mRoute := systray.AddMenuItem(title, fmt.Sprintf("路由: %s", route.PathPattern))

		// 保存路由菜单引用
		trayRouteMenus = append(trayRouteMenus, mRoute)

		// 保存该路由下的所有配置子菜单引用
		configMenus := make([]*systray.MenuItem, 0, len(allConfigs))
		for _, configName := range allConfigs {
			label := configName
			if configName == route.Config {
				label = "✓ " + configName
			}
			mConfig := mRoute.AddSubMenuItem(label, fmt.Sprintf("切换到配置: %s", configName))
			go handleConfigSwitch(mRoute, mConfig, i, route.PathPattern, configName)
			configMenus = append(configMenus, mConfig)
		}
		trayConfigMenus = append(trayConfigMenus, configMenus)
	}
}

// handleConfigSwitch 处理config切换点击事件
func handleConfigSwitch(mRoute *systray.MenuItem, mConfig *systray.MenuItem,
	routeIndex int, pathPattern string, targetConfig string) {
	for {
		<-mConfig.ClickedCh

		if err := trayConfigManager.SwitchRouteConfig(routeIndex, targetConfig); err != nil {
			log.Printf("切换config失败: %v", err)
			continue
		}

		log.Printf("路由 [%s] 已切换到config: %s", pathPattern, targetConfig)

		newConfig, err := trayConfigManager.ResolveConfig()
		if err != nil {
			log.Printf("解析新配置失败: %v", err)
			continue
		}

		trayProxy.ReloadConfig(newConfig)
		log.Printf("代理配置已热重载")

		if err := trayConfigManager.SaveMainConfig(); err != nil {
			log.Printf("保存配置文件失败: %v", err)
		}

		// 更新菜单标题
		mRoute.SetTitle(fmt.Sprintf("[%s] → %s", pathPattern, targetConfig))

		// 更新所有子菜单的打勾状态
		updateConfigMenuCheckmarks(routeIndex, targetConfig)
	}
}

// updateConfigMenuCheckmarks 更新指定路由的所有配置菜单项的打勾状态
func updateConfigMenuCheckmarks(routeIndex int, activeConfig string) {
	if routeIndex >= len(trayConfigMenus) {
		return
	}

	allConfigs := trayConfigManager.GetAllConfigNames()
	sort.Strings(allConfigs)

	for i, configName := range allConfigs {
		if i >= len(trayConfigMenus[routeIndex]) {
			continue
		}

		mConfig := trayConfigMenus[routeIndex][i]
		if configName == activeConfig {
			mConfig.SetTitle("✓ " + configName)
		} else {
			mConfig.SetTitle(configName)
		}
	}
}
