//go:build windows && 386

package abi

import (
	"syscall"
	"unsafe"
)

// U64 is a 64-bit value passed by value: two stack words, low first.
func U64(v uint64) []uintptr { return []uintptr{uintptr(v), uintptr(v >> 32)} }

// GUID is a GUID passed by value: its 16 bytes on the stack, as four words.
func GUID(g *syscall.GUID) []uintptr {
	w := (*[4]uint32)(unsafe.Pointer(g))
	return []uintptr{uintptr(w[0]), uintptr(w[1]), uintptr(w[2]), uintptr(w[3])}
}
