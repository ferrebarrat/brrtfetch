package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
)

type RAMSlide struct {
	mu       sync.RWMutex
	total    uint64
	used     uint64
	free     uint64
	buffers  uint64
	cached   uint64
	swap     uint64
	swapUsed uint64
	history  []float64
}

const ramHistoryLen = 60

func NewRAMSlide() *RAMSlide {
	s := &RAMSlide{history: make([]float64, 0, ramHistoryLen)}
	s.Refresh()
	return s
}

func (s *RAMSlide) Title() string      { return "Memory" }
func (s *RAMSlide) Icon() string       { return "󰍛" }
func (s *RAMSlide) NeedsRefresh() bool { return true }

func (s *RAMSlide) Refresh() {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return
	}
	defer f.Close()

	info := make(map[string]uint64)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		valStr := strings.TrimSpace(parts[1])
		valStr = strings.TrimSuffix(valStr, " kB")
		val, err := strconv.ParseUint(strings.TrimSpace(valStr), 10, 64)
		if err != nil {
			continue
		}
		info[key] = val * 1024
	}

	s.mu.Lock()
	s.total = info["MemTotal"]
	s.free = info["MemFree"]
	s.buffers = info["Buffers"]
	s.cached = info["Cached"]
	s.used = s.total - s.free - s.buffers - s.cached
	s.swap = info["SwapTotal"]
	s.swapUsed = info["SwapTotal"] - info["SwapFree"]

	if s.total > 0 {
		s.history = append(s.history, float64(s.used)/float64(s.total))
		if len(s.history) > ramHistoryLen {
			s.history = s.history[len(s.history)-ramHistoryLen:]
		}
	}
	s.mu.Unlock()
}

const ramLabelW = 10

func (s *RAMSlide) Render(width, height int) []string {
	s.mu.RLock()
	total := s.total
	used := s.used
	free := s.free
	buffers := s.buffers
	cached := s.cached
	swap := s.swap
	swapUsed := s.swapUsed
	history := make([]float64, len(s.history))
	copy(history, s.history)
	s.mu.RUnlock()

	lines := make([]string, 0, height)

	usedStr := fmt.Sprintf("%s / %s", formatBytes(used), formatBytes(total))
	lines = append(lines, renderRow("󰍛", "Used", usedStr, ramLabelW, width))
	lines = append(lines, renderRow("󰘚", "Free", formatBytes(free), ramLabelW, width))
	lines = append(lines, renderRow("󰓅", "Buffers", formatBytes(buffers), ramLabelW, width))
	lines = append(lines, renderRow("", "Cached", formatBytes(cached), ramLabelW, width))

	lines = append(lines, "")

	var ramUsage float64
	if total > 0 {
		ramUsage = float64(used) / float64(total)
	}
	lines = append(lines, renderUsageBar("󰍛", "RAM", ramUsage, ramLabelW, width))

	if swap > 0 {
		var swapUsage float64
		if swap > 0 {
			swapUsage = float64(swapUsed) / float64(swap)
		}
		lines = append(lines, renderUsageBar("󰾴", "Swap", swapUsage, ramLabelW, width))
	}

	// Compact graph (max 3 rows, right below the bars)
	if len(history) > 1 && height-len(lines) >= 3 {
		graphW := width - 4
		if graphW > len(history) {
			graphW = len(history)
		}
		graphH := 3
		if height-len(lines) < graphH {
			graphH = height - len(lines)
		}
		lines = append(lines, renderSparkGraph(history, graphW, graphH)...)
	}

	return lines
}
