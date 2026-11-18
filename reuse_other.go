//go:build !windows

package main

import "syscall"

// SO_REUSEPORT is what BSD/macOS need to share a UDP port for broadcasts.
func setReuse(fd uintptr) error {
	syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, soReusePort, 1)
	return syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
}
