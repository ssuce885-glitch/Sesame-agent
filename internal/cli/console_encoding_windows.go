//go:build windows

package cli

import "syscall"

func ensureConsoleUTF8() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	setConsoleCP := kernel32.NewProc("SetConsoleCP")
	setConsoleOutputCP := kernel32.NewProc("SetConsoleOutputCP")

	_ = configureConsoleUTF8(func(codePage uint32, output bool) error {
		var proc *syscall.LazyProc
		if output {
			proc = setConsoleOutputCP
		} else {
			proc = setConsoleCP
		}
		if err := proc.Find(); err != nil {
			return err
		}
		r1, _, err := proc.Call(uintptr(codePage))
		if r1 == 0 {
			return err
		}
		return nil
	})
}
