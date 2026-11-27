//go:build windows && !386

package abi

import (
	"syscall"
	"unsafe"
)

// U64 is a 64-bit value passed by value: one register or stack word.
func U64(v uint64) []uintptr { return []uintptr{uintptr(v)} }

// GUID is a GUID passed by value, which the 64-bit convention passes as a
// pointer to a copy.
func GUID(g *syscall.GUID) []uintptr { return []uintptr{uintptr(unsafe.Pointer(g))} }
