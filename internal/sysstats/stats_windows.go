//go:build windows

package sysstats

import (
	"syscall"

	"golang.org/x/sys/windows"
)

func getDiskStatsManual(path string) (DiskStats, error) {
	var total, free uint64

	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return DiskStats{}, err
	}

	// freeBytesAvailableToCaller is the free space the calling application can use.
	err = windows.GetDiskFreeSpaceEx(pathPtr, &free, &total, nil)
	if err != nil {
		return DiskStats{}, err
	}

	used := total - free
	usedPercent := 0.0
	if total > 0 {
		usedPercent = float64(used) / float64(total) * 100
	}

	return DiskStats{
		Path:        path,
		Total:       total,
		Used:        used,
		Free:        free,
		UsedPercent: usedPercent,
	}, nil
}