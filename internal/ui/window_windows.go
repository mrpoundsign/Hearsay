//go:build windows

package ui

import (
	"syscall"
	"unsafe"
)

var (
	procFindWindowW  = user32.NewProc("FindWindowW")
	procSetWindowPos = user32.NewProc("SetWindowPos")
)

const (
	hwndTopMost    = ^uintptr(0) // -1
	hwndNoTopMost  = ^uintptr(1) // -2
	swpNoSize      = 0x0001
	swpNoMove      = 0x0002
	swpNoActivate  = 0x0010
	swpShowWindow  = 0x0040
)

// SetWindowAlwaysOnTop sets or clears the Win32 WS_EX_TOPMOST flag on the window with the given title.
func SetWindowAlwaysOnTop(windowTitle string, topmost bool) {
	if windowTitle == "" {
		return
	}
	titlePtr, err := syscall.UTF16PtrFromString(windowTitle)
	if err != nil {
		return
	}
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(titlePtr)))
	if hwnd == 0 {
		return
	}
	insertAfter := hwndNoTopMost
	if topmost {
		insertAfter = hwndTopMost
	}
	_, _, _ = procSetWindowPos.Call(
		hwnd,
		insertAfter,
		0, 0, 0, 0,
		swpNoMove|swpNoSize|swpNoActivate|swpShowWindow,
	)
}
