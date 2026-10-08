// Command tars-desktop is a thin native shell around the TARS console.
//
// It shows the console served by a local TARS server in a native window and
// adds what a browser tab cannot: a tray icon that says whether a chat is
// running or waiting for approval, native notifications that approve or
// deny a tool call without opening the window, a global hotkey, tars://
// links, chats in windows of their own, and self-update from GitHub releases.
//
// The shell holds no state of its own beyond where its windows were and
// which chats had a window of their own. The
// server runs separately (the launchd service on macOS, a detached
// `tars serve` elsewhere) so closing or quitting the shell never stops a
// chat, a cron job, or pulse.
package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"runtime"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
	"github.com/wailsapp/wails/v3/pkg/updater"

	"github.com/devlikebear/tars/desktop/internal/deeplink"
	"github.com/devlikebear/tars/desktop/internal/icon"
	"github.com/devlikebear/tars/desktop/internal/protocol"
	"github.com/devlikebear/tars/desktop/internal/server"
	"github.com/devlikebear/tars/desktop/internal/serverupdate"
	"github.com/devlikebear/tars/desktop/internal/update"
)

// version is set at build time: -ldflags "-X main.version=0.38.0".
var version = "dev"

// DefaultHotkey shows or hides the window from anywhere.
const DefaultHotkey = "CmdOrCtrl+Alt+K"

//go:embed frontend
var frontend embed.FS

func main() {
	serverURL := flag.String("server-url", "", "TARS server URL (default $TARS_SERVER_URL or "+server.DefaultURL+"); loopback only")
	hotkey := flag.String("hotkey", "", `global show/hide shortcut (default $TARS_DESKTOP_HOTKEY or `+DefaultHotkey+`; "off" disables it)`)
	hidden := flag.Bool("hidden", false, "start with the window hidden (for login items)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("tars-desktop %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
		return
	}

	var file server.FileConfig
	if path, err := server.DefaultConfigPath(); err == nil {
		if file, err = server.LoadFile(path); err != nil {
			log.Printf("config file ignored: %v", err)
		}
	}
	cfg, err := server.Resolve(*serverURL, os.Getenv, file)
	if err != nil {
		log.Fatal(err)
	}
	if *hotkey == "" {
		*hotkey = strings.TrimSpace(os.Getenv("TARS_DESKTOP_HOTKEY"))
	}
	if *hotkey == "" {
		*hotkey = DefaultHotkey
	}

	offline, err := fs.Sub(frontend, "frontend")
	if err != nil {
		log.Fatal(err)
	}

	s := newShell(cfg)
	ns := notifications.New()
	s.notifier = ns

	app := application.New(application.Options{
		Name:        "TARS",
		Description: "Desktop shell for the TARS console",
		Icon:        icon.App(256),
		Services:    []application.Service{application.NewService(ns)},
		Assets:      application.AssetOptions{Handler: application.AssetFileServerFS(offline), DisableLogging: true},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID:               "com.devlikebear.tars.desktop",
			OnSecondInstanceLaunch: s.onSecondInstance,
		},
		ShouldQuit: func() bool {
			s.quitting.Store(true)
			return true
		},
		OnShutdown:        s.onShutdown,
		RawMessageHandler: s.onRawMessage,
		Mac: application.MacOptions{
			ActivationPolicy: application.ActivationPolicyRegular,
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		Windows: application.WindowsOptions{DisableQuitOnLastWindowClosed: true},
		Linux: application.LinuxOptions{
			DisableQuitOnLastWindowClosed: true,
			ProgramName:                   "tars-desktop",
		},
	})
	s.app = app

	// A winget install is upgraded by winget; replacing the exe in place would
	// leave winget's record of the installed version stale.
	if exe, _ := os.Executable(); serverupdate.ManagedByWinget(exe) {
		log.Print("updates disabled: installed with winget; run: winget upgrade Devlikebear.TARS.Desktop")
	} else if update.Enabled(version) {
		if provider, err := update.Provider(); err != nil {
			log.Printf("updates disabled: %v", err)
		} else if err := app.Updater.Init(updater.Config{
			CurrentVersion: strings.TrimPrefix(version, "v"),
			Providers:      []updater.Provider{provider},
		}); err != nil {
			log.Printf("updates disabled: %v", err)
		} else {
			s.updates = true
		}
	}

	s.createWindow(!*hidden)
	s.createTray()

	if !strings.EqualFold(*hotkey, "off") {
		if err := app.GlobalShortcut.Register(*hotkey, s.toggleWindow); err != nil {
			log.Printf("global shortcut %s: %v", *hotkey, err)
		}
	}

	// macOS delivers tars:// links and folders dropped on the Dock icon as
	// events; Windows and Linux start a second instance with them in argv,
	// which SingleInstance forwards to onSecondInstance.
	app.Event.OnApplicationEvent(events.Common.ApplicationLaunchedWithUrl, func(e *application.ApplicationEvent) {
		s.handleLink(e.Context().URL())
	})
	app.Event.OnApplicationEvent(events.Common.ApplicationOpenedWithFile, func(e *application.ApplicationEvent) {
		s.proposeChatIn(e.Context().Filename(), false)
	})
	app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(*application.ApplicationEvent) {
		s.showConsole("")
	})
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		s.start(flag.Args())
	})

	if exe, err := os.Executable(); err == nil {
		if changed, err := protocol.Register(exe); err != nil {
			log.Printf("register %s:// links: %v", deeplink.Scheme, err)
		} else if changed {
			log.Printf("registered %s:// links to %s", deeplink.Scheme, exe)
		}
	}

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
