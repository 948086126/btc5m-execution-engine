//go:build windows

package clock

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	qpcProc  = kernel32.NewProc("QueryPerformanceCounter")
	qpfProc  = kernel32.NewProc("QueryPerformanceFrequency")
	freqOnce sync.Once
	freq     int64
	freqErr  error
)

func loadFreq() {
	var f int64
	r, _, e := qpfProc.Call(uintptr(unsafe.Pointer(&f)))
	if r == 0 {
		freqErr = fmt.Errorf("QueryPerformanceFrequency: %v", e)
		return
	}
	if f <= 0 {
		freqErr = fmt.Errorf("invalid QueryPerformanceFrequency %d", f)
		return
	}
	freq = f
}

// MonotonicNS returns a QueryPerformanceCounter-derived monotonic timestamp,
// comparable across processes on the same Windows host.
func MonotonicNS() (int64, error) {
	freqOnce.Do(loadFreq)
	if freqErr != nil {
		return 0, freqErr
	}
	var c int64
	r, _, e := qpcProc.Call(uintptr(unsafe.Pointer(&c)))
	if r == 0 {
		return 0, fmt.Errorf("QueryPerformanceCounter: %v", e)
	}
	sec := c / freq
	rem := c % freq
	return sec*1_000_000_000 + rem*1_000_000_000/freq, nil
}
