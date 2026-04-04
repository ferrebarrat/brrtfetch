package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
)

type CPUSlide struct {
	mu                  sync.RWMutex
	model               string
	cores               int
	threads             int
	freqMHz             float64
	maxFreqMHz          float64
	usage               float64
	history             []float64
	prevIdle, prevTotal uint64
}

const cpuHistoryLen = 60

func NewCPUSlide() *CPUSlide {
	s := &CPUSlide{history: make([]float64, 0, cpuHistoryLen)}
	s.readStaticInfo()
	s.readUsage()
	return s
}

func (s *CPUSlide) Title() string      { return "CPU" }
func (s *CPUSlide) Icon() string       { return "󰻠" }
func (s *CPUSlide) NeedsRefresh() bool { return true }

func (s *CPUSlide) Refresh() {
	s.readUsage()
	s.readFreq()
}

const cpuLabelW = 10

func (s *CPUSlide) Render(width, height int) []string {
	s.mu.RLock()
	model := s.model
	cores := s.cores
	threads := s.threads
	freqMHz := s.freqMHz
	maxFreqMHz := s.maxFreqMHz
	usage := s.usage
	history := make([]float64, len(s.history))
	copy(history, s.history)
	s.mu.RUnlock()

	lines := make([]string, 0, height)

	if model != "" {
		lines = append(lines, renderRow("󰻠", "Model", model, cpuLabelW, width))
	}
	lines = append(lines, renderRow("󰍹", "Topology", fmt.Sprintf("%dC / %dT", cores, threads), cpuLabelW, width))
	if freqMHz > 0 {
		freq := formatFreq(freqMHz)
		if maxFreqMHz > 0 && maxFreqMHz != freqMHz {
			freq += cSep + " / " + formatFreq(maxFreqMHz) + " max" + cReset
		}
		lines = append(lines, renderRow("󰓅", "Frequency", freq, cpuLabelW, width))
	}

	lines = append(lines, "")
	lines = append(lines, renderUsageBar("󰘚", "Usage", usage, cpuLabelW, width))

	// Compact graph (max 3 rows, right below the bar)
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

func formatFreq(mhz float64) string {
	if mhz >= 1000 {
		return fmt.Sprintf("%.2f GHz", mhz/1000.0)
	}
	return fmt.Sprintf("%.0f MHz", mhz)
}

// ── CPU data gathering ─────────────────────────────────────────────

func (s *CPUSlide) readStaticInfo() {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	processors := 0
	coreIDs := make(map[string]bool)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "model name") && s.model == "" {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				s.model = strings.TrimSpace(parts[1])
			}
		}
		if strings.HasPrefix(line, "processor") {
			processors++
		}
		if strings.HasPrefix(line, "core id") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				coreIDs[strings.TrimSpace(parts[1])] = true
			}
		}
	}
	s.threads = processors
	if len(coreIDs) > 0 {
		s.cores = len(coreIDs)
	} else {
		s.cores = processors
	}

	// Max frequency
	data, err := os.ReadFile("/sys/devices/system/cpu/cpu0/cpufreq/cpuinfo_max_freq")
	if err == nil {
		if khz, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64); err == nil {
			s.maxFreqMHz = khz / 1000.0
		}
	}

	s.readFreq()
}

func (s *CPUSlide) readFreq() {
	data, err := os.ReadFile("/sys/devices/system/cpu/cpu0/cpufreq/scaling_cur_freq")
	if err == nil {
		if khz, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64); err == nil {
			s.mu.Lock()
			s.freqMHz = khz / 1000.0
			s.mu.Unlock()
			return
		}
	}
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "cpu MHz") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				if mhz, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64); err == nil {
					s.mu.Lock()
					s.freqMHz = mhz
					s.mu.Unlock()
					return
				}
			}
		}
	}
}

func (s *CPUSlide) readUsage() {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			return
		}
		var total, idle uint64
		for i := 1; i < len(fields); i++ {
			v, _ := strconv.ParseUint(fields[i], 10, 64)
			total += v
			if i == 4 {
				idle = v
			}
		}
		s.mu.Lock()
		if s.prevTotal > 0 {
			dt := total - s.prevTotal
			di := idle - s.prevIdle
			if dt > 0 {
				s.usage = 1.0 - float64(di)/float64(dt)
			}
			s.history = append(s.history, s.usage)
			if len(s.history) > cpuHistoryLen {
				s.history = s.history[len(s.history)-cpuHistoryLen:]
			}
		}
		s.prevIdle = idle
		s.prevTotal = total
		s.mu.Unlock()
		return
	}
}
