package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

type SystemSlide struct {
	mu   sync.RWMutex
	info map[string]string
}

func NewSystemSlide() *SystemSlide {
	s := &SystemSlide{info: make(map[string]string)}
	s.Refresh()
	return s
}

func (s *SystemSlide) Title() string      { return "System" }
func (s *SystemSlide) Icon() string       { return "" }
func (s *SystemSlide) NeedsRefresh() bool { return true }

func (s *SystemSlide) Refresh() {
	info := make(map[string]string)
	info["OS"] = readFileField("/etc/os-release", "PRETTY_NAME")
	info["Kernel"] = readCommandOutput("uname", "-r")
	info["Arch"] = runtime.GOARCH
	info["Hostname"] = readCommandOutput("hostname")
	info["Uptime"] = getUptime()
	info["Shell"] = getShellInfo()
	info["Terminal"] = getTerminalInfo()
	info["DE"] = getDE()
	info["WM"] = getWM()
	info["Packages"] = getPackageCount()

	s.mu.Lock()
	s.info = info
	s.mu.Unlock()
}

type sysField struct {
	icon, label, key string
}

var sysFields = []sysField{
	{"", "OS", "OS"},
	{"", "Kernel", "Kernel"},
	{"󰻠", "Arch", "Arch"},
	{"󰒋", "Host", "Hostname"},
	{"󰅐", "Uptime", "Uptime"},
	{"", "Shell", "Shell"},
	{"", "Term", "Terminal"},
	{"", "DE", "DE"},
	{"", "WM", "WM"},
	{"󰏗", "Pkgs", "Packages"},
}

const sysLabelW = 8 // all labels fit within this width

func (s *SystemSlide) Render(width, height int) []string {
	s.mu.RLock()
	info := s.info
	s.mu.RUnlock()

	// Filter out empty fields
	var active []sysField
	for _, f := range sysFields {
		if info[f.key] != "" {
			active = append(active, f)
		}
	}

	lines := make([]string, 0, height)

	if width >= 52 {
		// Two-column layout: each column is exactly half the width
		colW := width / 2

		mid := (len(active) + 1) / 2
		for i := 0; i < mid && len(lines) < height-3; i++ {
			left := renderRowFixed(active[i].icon, active[i].label, info[active[i].key], sysLabelW, colW)
			right := ""
			if i+mid < len(active) {
				right = renderRowFixed(active[i+mid].icon, active[i+mid].label, info[active[i+mid].key], sysLabelW, colW)
			}
			lines = append(lines, left+right)
		}
	} else {
		// Single column
		for _, f := range active {
			if len(lines) >= height-3 {
				break
			}
			lines = append(lines, renderRow(f.icon, f.label, info[f.key], sysLabelW, width))
		}
	}

	// Color palette at the bottom if we have room
	remaining := height - len(lines)
	if remaining >= 4 {
		lines = append(lines, "")
		palette := renderColorPalette()
		for _, pl := range strings.Split(palette, "\n") {
			lines = append(lines, pl)
		}
	}

	return lines
}

// ── System info gathering ──────────────────────────────────────────

func readFileField(path, key string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, key+"=") {
			val := strings.TrimPrefix(line, key+"=")
			return strings.Trim(val, "\"")
		}
	}
	return ""
}

func readCommandOutput(name string, args ...string) string {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func getUptime() string {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return readCommandOutput("uptime", "-p")
	}
	var secs float64
	fmt.Sscanf(string(data), "%f", &secs)
	d := int(secs) / 86400
	h := (int(secs) % 86400) / 3600
	m := (int(secs) % 3600) / 60
	if d > 0 {
		return fmt.Sprintf("%dd %dh %dm", d, h, m)
	}
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

func getShellInfo() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return ""
	}
	parts := strings.Split(shell, "/")
	name := parts[len(parts)-1]
	ver := readCommandOutput(shell, "--version")
	if ver != "" {
		if idx := strings.IndexByte(ver, '\n'); idx != -1 {
			ver = ver[:idx]
		}
		for _, word := range strings.Fields(ver) {
			if len(word) > 0 && word[0] >= '0' && word[0] <= '9' {
				return name + " " + word
			}
		}
	}
	return name
}

func getTerminalInfo() string {
	if t := os.Getenv("TERM_PROGRAM"); t != "" {
		if ver := os.Getenv("TERM_PROGRAM_VERSION"); ver != "" {
			return t + " " + ver
		}
		return t
	}
	if t := os.Getenv("TERM"); t != "" {
		return t
	}
	return ""
}

func getDE() string {
	if de := os.Getenv("XDG_CURRENT_DESKTOP"); de != "" {
		return de
	}
	if de := os.Getenv("DESKTOP_SESSION"); de != "" {
		return de
	}
	return ""
}

func getWM() string {
	wm := readCommandOutput("wmctrl", "-m")
	if wm != "" {
		for _, line := range strings.Split(wm, "\n") {
			if strings.HasPrefix(line, "Name:") {
				return strings.TrimSpace(strings.TrimPrefix(line, "Name:"))
			}
		}
	}
	for _, name := range []string{"sway", "hyprland", "i3", "bspwm", "dwm", "openbox", "awesome", "qtile", "river", "kwin", "mutter", "xfwm4", "marco"} {
		if readCommandOutput("pgrep", "-x", name) != "" {
			return name
		}
	}
	return ""
}

func getPackageCount() string {
	var counts []string
	if out := readCommandOutput("pacman", "-Qq"); out != "" {
		n := strings.Count(out, "\n")
		if !strings.HasSuffix(out, "\n") {
			n++
		}
		counts = append(counts, fmt.Sprintf("%d (pacman)", n))
	}
	if out := readCommandOutput("dpkg-query", "-f", "${binary:Package}\\n", "-W"); out != "" {
		n := strings.Count(out, "\n")
		counts = append(counts, fmt.Sprintf("%d (dpkg)", n))
	}
	if out := readCommandOutput("flatpak", "list", "--app"); out != "" {
		n := strings.Count(out, "\n")
		if !strings.HasSuffix(out, "\n") {
			n++
		}
		if n > 0 {
			counts = append(counts, fmt.Sprintf("%d (flatpak)", n))
		}
	}
	if out := readCommandOutput("snap", "list"); out != "" {
		n := strings.Count(out, "\n") - 1
		if n > 0 {
			counts = append(counts, fmt.Sprintf("%d (snap)", n))
		}
	}
	if len(counts) == 0 {
		return ""
	}
	return strings.Join(counts, ", ")
}
