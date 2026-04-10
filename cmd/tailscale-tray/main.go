// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

// tailscale-tray is a Windows system tray application for Tailscale-Custom.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"fyne.io/systray"
	"golang.org/x/sys/windows"
	"tailscale.com/client/local"
	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
)

// app holds the systray menu state.
type app struct {
	lc *local.Client

	mu          sync.Mutex
	status      *ipnstate.Status
	curProfile  ipn.LoginProfile
	allProfiles []ipn.LoginProfile

	bgCtx    context.Context
	bgCancel context.CancelFunc

	connect    *systray.MenuItem
	disconnect *systray.MenuItem
	quit       *systray.MenuItem

	rebuildCh   chan struct{}
	eventCancel context.CancelFunc
}

func main() {
	// Single instance check
	mutexName, _ := windows.UTF16PtrFromString("Global\\Tailscale-Custom-Tray-Mutex")
	handle, err := windows.CreateMutex(nil, false, mutexName)
	if err != nil {
		os.Exit(1)
	}
	if windows.GetLastError() == windows.ERROR_ALREADY_EXISTS {
		windows.CloseHandle(handle)
		os.Exit(0)
	}
	defer windows.CloseHandle(handle)

	setupLogging()
	log.Println("tailscale-tray starting")

	a := &app{
		lc:        &local.Client{},
		rebuildCh: make(chan struct{}, 1),
	}
	a.bgCtx, a.bgCancel = context.WithCancel(context.Background())
	defer a.bgCancel()

	a.updateState()
	go a.watchIPNBus()
	systray.Run(a.onReady, a.onExit)
}

func setupLogging() {
	dirs := []string{
		filepath.Join(os.Getenv("ProgramData"), "Tailscale-Custom", "Logs"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Tailscale-Custom", "Logs"),
	}
	for _, dir := range dirs {
		os.MkdirAll(dir, 0700)
		f, err := os.OpenFile(filepath.Join(dir, "tray.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err == nil {
			log.SetOutput(f)
			return
		}
	}
}

func (a *app) updateState() {
	a.mu.Lock()
	defer a.mu.Unlock()

	ctx, cancel := context.WithTimeout(a.bgCtx, 5*time.Second)
	defer cancel()

	var err error
	a.status, err = a.lc.Status(ctx)
	if err != nil {
		log.Printf("updateState: Status error: %v", err)
		a.status = nil
	}
	a.curProfile, a.allProfiles, err = a.lc.ProfileStatus(ctx)
	if err != nil {
		log.Printf("updateState: ProfileStatus error: %v", err)
	}
}

func (a *app) onReady() {
	log.Println("onReady")
	systray.SetIcon(iconDisconnected)
	systray.SetTooltip("Tailscale-Custom")
	a.rebuild()
}

func (a *app) onExit() {
	log.Println("onExit")
	a.bgCancel()
}

// onClick registers a per-item click handler in its own goroutine.
// This is the same pattern used by the official Tailscale systray.
func onClick(ctx context.Context, item *systray.MenuItem, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("PANIC in onClick: %v", r)
			}
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case <-item.ClickedCh:
				fn()
			}
		}
	}()
}

func (a *app) rebuild() {
	a.mu.Lock()
	defer a.mu.Unlock()

	log.Println("rebuild: start")

	if a.eventCancel != nil {
		a.eventCancel()
	}
	ctx, cancelFn := context.WithCancel(a.bgCtx)
	a.eventCancel = cancelFn

	systray.ResetMenu()

	// --- Status line ---
	var stateStr string
	if a.status == nil {
		stateStr = "Not connected"
		systray.SetIcon(iconDisconnected)
		systray.SetTooltip("Tailscale-Custom - Not Available")
	} else {
		switch a.status.BackendState {
		case "Running":
			if a.status.Self != nil && len(a.status.Self.TailscaleIPs) > 0 {
				stateStr = fmt.Sprintf("Connected: %s (%s)", a.status.Self.HostName, a.status.Self.TailscaleIPs[0])
			} else {
				stateStr = "Connected"
			}
			systray.SetIcon(iconConnected)
			systray.SetTooltip("Tailscale-Custom - Connected")
		case "NeedsLogin":
			stateStr = "Login required"
			systray.SetIcon(iconDisconnected)
		default:
			stateStr = "Disconnected"
			systray.SetIcon(iconDisconnected)
			systray.SetTooltip("Tailscale-Custom - Disconnected")
		}
	}
	statusItem := systray.AddMenuItem(stateStr, "")
	statusItem.Disable()

	systray.AddSeparator()

	// --- Connect / Disconnect ---
	a.connect = systray.AddMenuItem("Connect", "Connect to Tailscale")
	a.disconnect = systray.AddMenuItem("Disconnect", "Disconnect from Tailscale")
	a.disconnect.Hide()

	if a.status != nil && a.status.BackendState == "Running" {
		a.connect.SetTitle("Connected")
		a.connect.Disable()
		a.disconnect.Show()
		a.disconnect.Enable()
	}

	onClick(ctx, a.connect, func() {
		log.Println("action: Connect")
		opCtx, opCancel := context.WithTimeout(a.bgCtx, 10*time.Second)
		defer opCancel()
		_, err := a.lc.EditPrefs(opCtx, &ipn.MaskedPrefs{
			Prefs:          ipn.Prefs{WantRunning: true},
			WantRunningSet: true,
		})
		if err != nil {
			log.Printf("Connect error: %v", err)
		} else {
			log.Println("Connect: OK")
		}
	})

	onClick(ctx, a.disconnect, func() {
		log.Println("action: Disconnect")
		opCtx, opCancel := context.WithTimeout(a.bgCtx, 10*time.Second)
		defer opCancel()
		_, err := a.lc.EditPrefs(opCtx, &ipn.MaskedPrefs{
			Prefs:          ipn.Prefs{WantRunning: false},
			WantRunningSet: true,
		})
		if err != nil {
			log.Printf("Disconnect error: %v", err)
		} else {
			log.Println("Disconnect: OK")
		}
	})

	systray.AddSeparator()

	// --- Profiles (flat top-level items, same as official systray) ---
	if len(a.allProfiles) > 0 {
		accountLabel := "Account"
		if a.curProfile.Name != "" {
			accountLabel = a.curProfile.Name
		}
		accounts := systray.AddMenuItem(accountLabel, "")
		time.Sleep(10 * time.Millisecond) // workaround for systray submenu race

		for _, profile := range a.allProfiles {
			title := profileTitle(profile)
			var item *systray.MenuItem
			if profile.ID == a.curProfile.ID {
				item = accounts.AddSubMenuItemCheckbox(title, "", true)
			} else {
				item = accounts.AddSubMenuItem(title, "")
			}
			pid := profile.ID
			onClick(ctx, item, func() {
				log.Printf("action: switch profile %v", pid)
				opCtx, opCancel := context.WithTimeout(a.bgCtx, 10*time.Second)
				defer opCancel()
				if err := a.lc.SwitchProfile(opCtx, pid); err != nil {
					log.Printf("SwitchProfile error: %v", err)
				} else {
					log.Println("SwitchProfile: OK")
				}
			})
		}
	}

	systray.AddSeparator()

	// --- Add Server ---
	addServerItem := systray.AddMenuItem("Add Server...", "Connect to a new control server")
	onClick(ctx, addServerItem, func() {
		log.Println("action: Add Server")
		a.addServer()
	})

	systray.AddSeparator()

	// --- Quit ---
	a.quit = systray.AddMenuItem("Quit", "Quit")
	onClick(ctx, a.quit, func() {
		log.Println("action: Quit")
		systray.Quit()
	})

	// --- Rebuild listener ---
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("PANIC in rebuild listener: %v", r)
			}
		}()
		select {
		case <-ctx.Done():
			return
		case <-a.rebuildCh:
			log.Println("rebuild triggered by IPNBus")
			a.updateState()
			a.rebuild()
		}
	}()

	log.Printf("rebuild: done (state=%q, profiles=%d)", stateStr, len(a.allProfiles))
}

func (a *app) addServer() {
	serverURL := inputDialog("Add Server", "Enter the control server URL (e.g. https://vpn.softs.business)")
	if serverURL == "" {
		log.Println("addServer: cancelled")
		return
	}
	if !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
		serverURL = "https://" + serverURL
	}
	log.Printf("addServer: url=%s", serverURL)

	opCtx, opCancel := context.WithTimeout(a.bgCtx, 15*time.Second)
	defer opCancel()

	if err := a.lc.SwitchToEmptyProfile(opCtx); err != nil {
		log.Printf("addServer: SwitchToEmptyProfile error: %v", err)
		showError("Failed to create profile: " + err.Error())
		return
	}

	_, err := a.lc.EditPrefs(opCtx, &ipn.MaskedPrefs{
		Prefs: ipn.Prefs{
			ControlURL:  serverURL,
			WantRunning: true,
		},
		ControlURLSet:  true,
		WantRunningSet: true,
	})
	if err != nil {
		log.Printf("addServer: EditPrefs error: %v", err)
		showError("Failed to set server: " + err.Error())
		return
	}

	if err := a.lc.StartLoginInteractive(opCtx); err != nil {
		log.Printf("addServer: StartLoginInteractive error: %v", err)
		tailscaleExe := findTailscaleExe()
		exec.Command(tailscaleExe, "login", "--login-server", serverURL).Start()
	}
	log.Println("addServer: done")
	a.triggerRebuild()
}

func (a *app) watchIPNBus() {
	for {
		err := a.watchIPNBusInner()
		if err != nil {
			log.Printf("watchIPNBus error: %v", err)
		}
		select {
		case <-a.bgCtx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func (a *app) watchIPNBusInner() error {
	watcher, err := a.lc.WatchIPNBus(a.bgCtx, 0)
	if err != nil {
		return err
	}
	defer watcher.Close()
	for {
		select {
		case <-a.bgCtx.Done():
			return nil
		default:
		}
		n, err := watcher.Next()
		if err != nil {
			return err
		}
		if n.State != nil || n.Prefs != nil {
			a.triggerRebuild()
		}
		if url := n.BrowseToURL; url != nil {
			exec.Command("rundll32", "url.dll,FileProtocolHandler", *url).Start()
		}
	}
}

func (a *app) triggerRebuild() {
	select {
	case a.rebuildCh <- struct{}{}:
	default:
	}
}

func profileTitle(p ipn.LoginProfile) string {
	name := p.Name
	if name == "" {
		name = "(new profile)"
	}
	if p.NetworkProfile.DomainName != "" {
		name += " (" + p.NetworkProfile.DisplayNameOrDefault() + ")"
	}
	if p.ControlURL != "" {
		u := strings.TrimPrefix(p.ControlURL, "https://")
		u = strings.TrimPrefix(u, "http://")
		u = strings.TrimSuffix(u, "/")
		name += " [" + u + "]"
	}
	return name
}

func findTailscaleExe() string {
	exe, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "tailscale.exe")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "tailscale.exe"
}

// ============ Windows Dialogs ============

var (
	user32                   = windows.NewLazySystemDLL("user32.dll")
	pMessageBoxW             = user32.NewProc("MessageBoxW")
	pDialogBoxIndirectParamW = user32.NewProc("DialogBoxIndirectParamW")
	pEndDialog               = user32.NewProc("EndDialog")
	pGetDlgItemTextW         = user32.NewProc("GetDlgItemTextW")
	pSetDlgItemTextW         = user32.NewProc("SetDlgItemTextW")
	pGetDlgItem              = user32.NewProc("GetDlgItem")
	pSendMessageW            = user32.NewProc("SendMessageW")
	pSetForegroundWindow     = user32.NewProc("SetForegroundWindow")
)

var (
	dlgInputResult string
	inputDlgCb     = windows.NewCallback(inputDlgProcFn)
)

func showError(msg string) {
	log.Printf("ERROR: %s", msg)
	title, _ := windows.UTF16PtrFromString("Tailscale-Custom")
	text, _ := windows.UTF16PtrFromString(msg)
	pMessageBoxW.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)),
		uintptr(0x00000000|0x00000010)) // MB_OK | MB_ICONERROR
}

func inputDlgProcFn(hwnd, msg, wParam, lParam uintptr) uintptr {
	const (
		wmInitDialog = 0x0110
		wmCommand    = 0x0111
		wmClose      = 0x0010
		idEdit       = 101
		emSetSel     = 0x00B1
	)
	switch msg {
	case wmInitDialog:
		pSetForegroundWindow.Call(hwnd)
		defText, _ := windows.UTF16PtrFromString("https://")
		pSetDlgItemTextW.Call(hwnd, idEdit, uintptr(unsafe.Pointer(defText)))
		editHwnd, _, _ := pGetDlgItem.Call(hwnd, idEdit)
		pSendMessageW.Call(editHwnd, emSetSel, 8, 8)
		return 1
	case wmCommand:
		switch int(wParam & 0xFFFF) {
		case 1: // IDOK
			buf := make([]uint16, 512)
			pGetDlgItemTextW.Call(hwnd, idEdit, uintptr(unsafe.Pointer(&buf[0])), 512)
			dlgInputResult = windows.UTF16ToString(buf)
			pEndDialog.Call(hwnd, 1)
		case 2: // IDCANCEL
			dlgInputResult = ""
			pEndDialog.Call(hwnd, 0)
		}
	case wmClose:
		dlgInputResult = ""
		pEndDialog.Call(hwnd, 0)
	}
	return 0
}

func inputDialog(title, prompt string) string {
	log.Printf("inputDialog: title=%q", title)

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	dlgInputResult = ""
	tmpl := buildInputDialogTemplate(title, prompt)

	ret, _, _ := pDialogBoxIndirectParamW.Call(
		0,
		uintptr(unsafe.Pointer(&tmpl[0])),
		0,
		inputDlgCb,
		0,
	)

	log.Printf("inputDialog: result=%q ret=%d", dlgInputResult, ret)
	if ret == 0 {
		return ""
	}
	result := strings.TrimSpace(dlgInputResult)
	if result == "" || result == "https://" {
		return ""
	}
	return result
}

type dlgBuilder struct{ buf []byte }

func (d *dlgBuilder) align(n int) {
	for len(d.buf)%n != 0 {
		d.buf = append(d.buf, 0)
	}
}

func (d *dlgBuilder) w16(v uint16) {
	d.buf = append(d.buf, byte(v), byte(v>>8))
}

func (d *dlgBuilder) w32(v uint32) {
	d.buf = append(d.buf, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}

func (d *dlgBuilder) ws16(v int16) { d.w16(uint16(v)) }

func (d *dlgBuilder) wstr(s string) {
	for _, c := range s {
		d.w16(uint16(c))
	}
	d.w16(0)
}

func buildInputDialogTemplate(title, prompt string) []byte {
	const (
		wsChild    = 0x40000000
		wsVisible  = 0x10000000
		wsCaption  = 0x00C00000
		wsSysMenu  = 0x00080000
		wsPopup    = 0x80000000
		bsPushBtn  = 0x00000000
		wsTabStop  = 0x00010000
		wsBorder   = 0x00800000
		esAutoHS   = 0x00000080
		dsSetFont  = 0x00000040
		dsModalFrm = 0x00000080
		ds3DLook   = 0x00000004
	)
	d := &dlgBuilder{}

	// DLGTEMPLATE
	d.w32(wsPopup | wsCaption | wsSysMenu | dsSetFont | dsModalFrm | ds3DLook) // style
	d.w32(0)                                                                     // exstyle
	d.w16(4)                                                                     // cdit (4 controls)
	d.ws16(0); d.ws16(0); d.ws16(250); d.ws16(85)                               // x, y, cx, cy
	d.w16(0)                                                                     // menu
	d.w16(0)                                                                     // class
	d.wstr(title)                                                                // title
	d.w16(9)                                                                     // font size
	d.wstr("Segoe UI")                                                           // font name

	// Static label id=100
	d.align(4)
	d.w32(wsChild | wsVisible); d.w32(0)
	d.ws16(10); d.ws16(10); d.ws16(230); d.ws16(14)
	d.w16(100); d.w16(0xFFFF); d.w16(0x0082)
	d.wstr(prompt); d.w16(0)

	// Edit id=101
	d.align(4)
	d.w32(wsChild | wsVisible | wsTabStop | wsBorder | esAutoHS); d.w32(0)
	d.ws16(10); d.ws16(30); d.ws16(230); d.ws16(14)
	d.w16(101); d.w16(0xFFFF); d.w16(0x0081)
	d.w16(0); d.w16(0)

	// OK id=1
	d.align(4)
	d.w32(wsChild | wsVisible | wsTabStop | bsPushBtn); d.w32(0)
	d.ws16(127); d.ws16(58); d.ws16(50); d.ws16(14)
	d.w16(1); d.w16(0xFFFF); d.w16(0x0080)
	d.wstr("OK"); d.w16(0)

	// Cancel id=2
	d.align(4)
	d.w32(wsChild | wsVisible | wsTabStop); d.w32(0)
	d.ws16(183); d.ws16(58); d.ws16(50); d.ws16(14)
	d.w16(2); d.w16(0xFFFF); d.w16(0x0080)
	d.wstr("Cancel"); d.w16(0)

	return d.buf
}
