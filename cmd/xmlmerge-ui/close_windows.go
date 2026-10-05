//go:build windows

package main

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"syscall"

	webview "github.com/jchv/go-webview2"
)

// Route the native X / Alt+F4 through the catalog's unsaved-change prompt.
// The server's approved close bypasses the hook before Terminate posts WM_CLOSE.
func guardCatalogClose(w webview.WebView) (allow func(), cleanup func(), err error) {
	var ready, approved atomic.Bool
	if err := w.Bind("xmlmergeEditorReady", func() { ready.Store(true) }); err != nil {
		return nil, nil, err
	}
	comctl := syscall.NewLazyDLL("comctl32.dll")
	set := comctl.NewProc("SetWindowSubclass")
	remove := comctl.NewProc("RemoveWindowSubclass")
	next := comctl.NewProc("DefSubclassProc")
	hwnd := uintptr(w.Window())
	callback := syscall.NewCallback(func(hwnd, message, wp, lp, id, data uintptr) uintptr {
		if message == 0x0010 && ready.Load() && !approved.Load() {
			w.Eval("window.xmlmergeRequestClose()")
			return 0
		}
		result, _, _ := next.Call(hwnd, message, wp, lp)
		return result
	})
	ok, _, callErr := set.Call(hwnd, callback, 1, 0)
	if ok == 0 {
		return nil, nil, fmt.Errorf("не удалось установить обработчик закрытия окна: %v", callErr)
	}
	return func() { approved.Store(true) }, func() {
		remove.Call(hwnd, callback, 1)
		runtime.KeepAlive(w)
	}, nil
}
