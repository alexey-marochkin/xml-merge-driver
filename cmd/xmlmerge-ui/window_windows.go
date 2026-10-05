//go:build windows

package main

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
	"unsafe"

	webview "github.com/jchv/go-webview2"
	"github.com/jchv/go-webview2/webviewloader"

	"xmlmerge/internal/ui"
)

func runWindow(server *ui.Server) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if version, err := webviewloader.GetInstalledVersion(); err != nil || version == "" {
		return fmt.Errorf("для окна XML Merge требуется Microsoft Edge WebView2 Runtime: %v", err)
	}
	profile, err := os.MkdirTemp("", "xmlmerge-webview-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(profile)
	w := webview.NewWithOptions(webview.WebViewOptions{DataPath: profile, AutoFocus: true, WindowOptions: webview.WindowOptions{Title: "XML Merge — правила соответствия", Width: 1280, Height: 900, Center: true, IconId: 1}})
	if w == nil {
		return fmt.Errorf("не удалось создать окно WebView2")
	}
	defer w.Destroy()
	allowClose := func() {}
	if server.Session.Options.StandaloneRules() {
		allow, cleanup, err := guardCatalogClose(w)
		if err != nil {
			return err
		}
		allowClose = allow
		defer cleanup()
	}
	w.SetSize(960, 680, webview.HintMin)
	w.Navigate(server.URL + "/#" + server.Token)
	stopped := make(chan struct{})
	go func() {
		select {
		case <-server.Done:
			w.Dispatch(func() { allowClose(); w.Terminate() })
		case <-stopped:
		}
	}()
	w.Run()
	close(stopped)
	server.Close()
	return nil
}

func showError(err error) {
	diagnostic(err)
	text, _ := syscall.UTF16PtrFromString(err.Error())
	title, _ := syscall.UTF16PtrFromString("XML Merge")
	_, _, _ = syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10)
}
