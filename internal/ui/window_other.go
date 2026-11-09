//go:build !windows

package ui

// SetWindowAlwaysOnTop is a no-op on non-Windows platforms.
func SetWindowAlwaysOnTop(_ string, _ bool) {}
