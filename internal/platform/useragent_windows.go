//go:build windows

package platform

import (
	"fmt"
	"syscall"
	"unsafe"
)

// getOSVersion returns Windows version (e.g., "10.0.26100")
func getOSVersion() string {
	dll := syscall.NewLazyDLL("ntdll.dll")
	proc := dll.NewProc("RtlGetVersion")

	type osVersionInfoEx struct {
		dwOSVersionInfoSize uint32
		dwMajorVersion      uint32
		dwMinorVersion      uint32
		dwBuildNumber       uint32
		dwPlatformId        uint32
		szCSDVersion        [128]uint16
	}

	var info osVersionInfoEx
	info.dwOSVersionInfoSize = uint32(unsafe.Sizeof(info))

	ret, _, _ := proc.Call(uintptr(unsafe.Pointer(&info)))
	if ret != 0 {
		return ""
	}

	return fmt.Sprintf("%d.%d.%d", info.dwMajorVersion, info.dwMinorVersion, info.dwBuildNumber)
}
