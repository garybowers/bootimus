package sysstats

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
)

// Stats represents system statistics
type Stats struct {
	CPU       CPUStats       `json:"cpu"`
	Memory    MemoryStats    `json:"memory"`
	Disk      []DiskStats    `json:"disk"`
	Host      HostInfo       `json:"host"`
	Timestamp time.Time      `json:"timestamp"`
	Uptime    string         `json:"uptime"`
}

// CPUStats represents CPU statistics
type CPUStats struct {
	UsagePercent float64 `json:"usage_percent"`
	Cores        int     `json:"cores"`
}

// MemoryStats represents memory statistics
type MemoryStats struct {
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Free        uint64  `json:"free"`
	UsedPercent float64 `json:"used_percent"`
}

// DiskStats represents disk statistics
type DiskStats struct {
	Path        string  `json:"path"`
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Free        uint64  `json:"free"`
	UsedPercent float64 `json:"used_percent"`
}

// HostInfo represents host system information
type HostInfo struct {
	OS              string `json:"os"`
	Platform        string `json:"platform"`
	PlatformVersion string `json:"platform_version"`
	Architecture    string `json:"architecture"`
}

var startTime = time.Now()

// GetStats retrieves current system statistics
func GetStats(paths []string) (*Stats, error) {
	stats := &Stats{
		Timestamp: time.Now(),
		Uptime:    formatUptime(time.Since(startTime)),
	}

	// Get host information
	hostInfo, err := host.Info()
	if err == nil {
		stats.Host = HostInfo{
			OS:              hostInfo.OS,
			Platform:        hostInfo.Platform,
			PlatformVersion: hostInfo.PlatformVersion,
			Architecture:    hostInfo.KernelArch,
		}
	} else {
		// Fallback to runtime info if host.Info fails
		stats.Host = HostInfo{
			OS:           runtime.GOOS,
			Architecture: runtime.GOARCH,
		}
	}

	// Get CPU stats
	cpuPercent, err := cpu.Percent(time.Second, false)
	if err == nil && len(cpuPercent) > 0 {
		stats.CPU.UsagePercent = cpuPercent[0]
	}
	stats.CPU.Cores = runtime.NumCPU()

	// Get memory stats
	vmStat, err := mem.VirtualMemory()
	if err == nil {
		stats.Memory.Total = vmStat.Total
		stats.Memory.Used = vmStat.Used
		stats.Memory.Free = vmStat.Available
		stats.Memory.UsedPercent = vmStat.UsedPercent
	}

	// Get disk stats for specified paths
	for _, path := range paths {
		diskStat, err := getDiskStats(path)
		if err == nil {
			stats.Disk = append(stats.Disk, diskStat)
		}
	}

	return stats, nil
}

// getDiskStats gets disk usage for a specific path
func getDiskStats(path string) (DiskStats, error) {
	usage, err := disk.Usage(path)
	if err != nil {
		return DiskStats{}, err
	}

	return DiskStats{
		Path:        path,
		Total:       usage.Total,
		Used:        usage.Used,
		Free:        usage.Free,
		UsedPercent: usage.UsedPercent,
	}, nil
}

// getDiskStatsManual is a fallback method using syscall
func getDiskStatsManual(path string) (DiskStats, error) {
	var stat syscall.Statfs_t
	err := syscall.Statfs(path, &stat)
	if err != nil {
		return DiskStats{}, err
	}

	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
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

// formatUptime formats duration into human-readable uptime
func formatUptime(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm %ds", days, hours, minutes, seconds)
	} else if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	} else if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// FormatBytes formats bytes into human-readable format
func FormatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// GetMonitoredPaths returns paths to monitor based on configuration
func GetMonitoredPaths(dataDir string) []string {
	paths := []string{"/"}

	// Add data directory if it's on a different mount
	if dataDir != "" {
		if _, err := os.Stat(dataDir); err == nil {
			paths = append(paths, dataDir)
		}
	}

	return paths
}
