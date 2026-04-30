//go:build windows

package cli

import "syscall"

func ensureConsoleUTF8() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	setConsoleCP := kernel32.NewProc("SetConsoleCP")
	setConsoleOutputCP := kernel32.NewProc("SetConsoleOutputCP")

	for _, output := range []bool{false, true} {
		proc := setConsoleCP
		if output {
			proc = setConsoleOutputCP
		}
		if err := proc.Find(); err != nil {
			return
		}
		r1, _, _ := proc.Call(uintptr(65001))
		if r1 == 0 {
			return
		}
	}
}
