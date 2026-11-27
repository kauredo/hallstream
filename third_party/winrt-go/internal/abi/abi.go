// Package abi lays out by-value arguments the way the Windows calling
// convention of each architecture expects them in a syscall argument list.
// Patched in by Hallstream: upstream passes them as one word, which is only
// right on 64-bit Windows.
package abi
