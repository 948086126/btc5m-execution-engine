//go:build linux

package clock

import (
	"fmt"
	"syscall"
	"unsafe"
)

const clockMonotonic = 1

// MonotonicNS returns CLOCK_MONOTONIC nanoseconds, comparable across processes
// on the same Linux host.
func MonotonicNS() (int64, error) {
	var ts syscall.Timespec
	_, _, errno := syscall.Syscall(syscall.SYS_CLOCK_GETTIME, uintptr(clockMonotonic), uintptr(unsafe.Pointer(&ts)), 0)
	if errno != 0 {
		return 0, fmt.Errorf("clock_gettime(CLOCK_MONOTONIC): %v", errno)
	}
	return ts.Sec*1_000_000_000 + ts.Nsec, nil
}
