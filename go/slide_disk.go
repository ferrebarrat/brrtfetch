package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
	"syscall"
)

type DiskInfo struct {
	Mount  string
	Device string
	FSType string
	Total  uint64
	Used   uint64
	Free   uint64
}

type DiskSlide struct {
	mu    sync.RWMutex
	disks []DiskInfo
}

func NewDiskSlide() *DiskSlide {
	s := &DiskSlide{}
	s.Refresh()
	return s
}

func (s *DiskSlide) Title() string      { return "Disk" }
func (s *DiskSlide) Icon() string       { return "" }
func (s *DiskSlide) NeedsRefresh() bool { return true }

func (s *DiskSlide) Refresh() {
	disks := readMounts()
	s.mu.Lock()
	s.disks = disks
	s.mu.Unlock()
}

const diskLabelW = 10

func (s *DiskSlide) Render(width, height int) []string {
	s.mu.RLock()
	disks := make([]DiskInfo, len(s.disks))
	copy(disks, s.disks)
	s.mu.RUnlock()

	lines := make([]string, 0, height)

	if len(disks) == 0 {
		lines = append(lines, cSep+"  No disks found"+cReset)
		return lines
	}

	for i, d := range disks {
		if len(lines) >= height-3 {
			break
		}

		var usage float64
		if d.Total > 0 {
			usage = float64(d.Used) / float64(d.Total)
		}

		// Mount point as a mini-header
		mountIcon := ""
		if d.Mount == "/" {
			mountIcon = ""
		} else if strings.HasPrefix(d.Mount, "/home") {
			mountIcon = ""
		} else if strings.HasPrefix(d.Mount, "/boot") {
			mountIcon = "󰒓"
		}
		header := fmt.Sprintf("  %s%s%s %s%s%s  %s%s  %s%s",
			cIcon, mountIcon, cReset,
			cLabel, d.Mount, cReset,
			cSep, d.FSType,
			d.Device, cReset)
		lines = append(lines, header)

		sizeStr := fmt.Sprintf("%s / %s", formatBytes(d.Used), formatBytes(d.Total))
		lines = append(lines, renderUsageBar("󰋊", "Used", usage, diskLabelW, width))
		lines = append(lines, renderRow("󰘚", "Free", formatBytes(d.Free), diskLabelW, width))
		lines = append(lines, renderRow("󰗮", "Size", sizeStr, diskLabelW, width))

		if i < len(disks)-1 && len(lines) < height-1 {
			lines = append(lines, "")
		}
	}

	return lines
}

func readMounts() []DiskInfo {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return nil
	}
	defer f.Close()

	seen := make(map[string]bool)
	var disks []DiskInfo

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		dev := fields[0]
		mount := fields[1]
		fstype := fields[2]

		if !strings.HasPrefix(dev, "/dev/") {
			continue
		}
		if strings.HasPrefix(dev, "/dev/loop") {
			continue
		}
		if seen[dev] {
			continue
		}
		seen[dev] = true

		var stat syscall.Statfs_t
		if err := syscall.Statfs(mount, &stat); err != nil {
			continue
		}

		total := stat.Blocks * uint64(stat.Bsize)
		free := stat.Bavail * uint64(stat.Bsize)
		used := total - free
		if total == 0 {
			continue
		}

		disks = append(disks, DiskInfo{
			Mount:  mount,
			Device: dev,
			FSType: fstype,
			Total:  total,
			Used:   used,
			Free:   free,
		})
	}

	return disks
}
