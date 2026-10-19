//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// enableANSI turns on ANSI escape sequences for a Windows console, which
// Windows 10 and later support only when asked to. It reports false when f
// is not a console or the console refuses, and the output then stays plain.
func enableANSI(f *os.File) bool {
	h := windows.Handle(f.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return false
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return true
	}
	return windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}
