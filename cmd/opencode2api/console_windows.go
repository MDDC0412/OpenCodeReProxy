//go:build windows

package main

import (
	"os"
	"syscall"
	"unsafe"
)

var getConsoleProcessList = syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleProcessList")

func ownsConsole() bool {
	var processes [2]uint32
	count, _, _ := getConsoleProcessList.Call(uintptr(unsafe.Pointer(&processes[0])), uintptr(len(processes)))
	// Explorer-created consoles contain only this process. Existing cmd,
	// PowerShell, or terminal sessions contain their shell as well; detached
	// services and redirected test processes have no console at all.
	return count == 1 && processes[0] == uint32(os.Getpid())
}
