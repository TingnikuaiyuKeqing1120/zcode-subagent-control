package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var trayIcon []byte

func main() {
	api := NewAPI()

	// 单实例：重复双击只唤出已有窗口，不开第二个进程
	var mainWinRef *application.WebviewWindow
	showMain := func() {
		if mainWinRef != nil {
			mainWinRef.Show()
			mainWinRef.Center()
		}
	}

	app := application.New(application.Options{
		// Name 会派生 WebView2 用户数据目录名，必须用 ASCII；中文只放 Title
		Name:        "zcode-subagent-control",
		Description: "管理 ZCode 自定义子智能体（~/.zcode/agents/*.md）",
		Services: []application.Service{
			application.NewService(api),
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "zcode-subagent-control",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				showMain()
			},
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	// 主窗口
	mainWin := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "main",
		Title:            "ZCode 子智能体控制台",
		Width:            1280,
		Height:           840,
		MinWidth:         1080,
		MinHeight:        680,
		BackgroundColour: application.NewRGB(13, 16, 23),
		URL:              "/",
	})
	mainWinRef = mainWin

	// 悬浮速切面板：无边框置顶小窗，默认隐藏，托盘唤出
	floatWin := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "float",
		Title:            "子智能体速切",
		Width:            400,
		Height:            600,
		Frameless:        true,
		AlwaysOnTop:      true,
		DisableResize:    true,
		Hidden:           true,
		BackgroundColour: application.NewRGB(13, 16, 23),
		URL:              "/float.html",
		// 无边框窗口的拖动：整窗 drag 区域 + 原生非客户区命中
		Windows: application.WindowsWindow{
			NonClientRegionSupport: true,
		},
	})

	quitting := false
	toggleFloat := func() { api.ToggleFloatPanel() }

	// 关闭主窗口 = 收回托盘，不退出；退出走托盘菜单
	mainWin.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if quitting {
			return
		}
		e.Cancel()
		mainWin.Hide()
	})

	// 托盘
	tray := app.SystemTray.New()
	tray.SetIcon(trayIcon)
	tray.SetTooltip("ZCode 子智能体控制台")
	trayMenu := application.NewMenu()
	trayMenu.Add("悬浮速切面板").OnClick(func(*application.Context) { toggleFloat() })
	trayMenu.Add("打开主窗口").OnClick(func(*application.Context) {
		mainWin.Show()
		mainWin.Center()
	})
	trayMenu.AddSeparator()
	trayMenu.Add("全部 → 完全访问").OnClick(func(*application.Context) {
		if _, err := api.SetAllPermissionModes("yolo"); err != nil {
			log.Printf("托盘批量切换失败: %v", err)
		}
	})
	trayMenu.Add("全部 → 变更前确认").OnClick(func(*application.Context) {
		if _, err := api.SetAllPermissionModes("default"); err != nil {
			log.Printf("托盘批量切换失败: %v", err)
		}
	})
	trayMenu.AddSeparator()
	trayMenu.Add("退出").OnClick(func(*application.Context) {
		quitting = true
		app.Quit()
	})
	tray.SetMenu(trayMenu)
	// 左键单击托盘图标 = 唤出/收起悬浮面板
	tray.OnClick(func() { toggleFloat() })

	// 窗口引用注入 API（前端关闭/唤出悬浮面板用）
	api.mainWin = mainWin
	api.floatWin = floatWin

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
