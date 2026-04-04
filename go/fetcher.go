package main

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Slide renders its content into lines that fit within the given width/height.
type Slide interface {
	Title() string
	Icon() string
	Render(width, height int) []string
	NeedsRefresh() bool
	Refresh()
}

// SlideShow manages slides and navigation state.
type SlideShow struct {
	slides      []Slide
	current     int
	mu          sync.RWMutex
	InfoXOffset int
}

func NewSlideShow(slides []Slide) *SlideShow {
	return &SlideShow{slides: slides}
}

func (ss *SlideShow) Current() int {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.current
}

func (ss *SlideShow) Total() int { return len(ss.slides) }

func (ss *SlideShow) Next() {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if ss.current < len(ss.slides)-1 {
		ss.current++
	}
}

func (ss *SlideShow) Prev() {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if ss.current > 0 {
		ss.current--
	}
}

func (ss *SlideShow) SetSlide(i int) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if i >= 0 && i < len(ss.slides) {
		ss.current = i
	}
}

// ── Color palette ──────────────────────────────────────────────────
// Uses base 16 ANSI colors so everything follows the terminal's theme.
const (
	cReset    = "\033[0m"
	cBold     = "\033[1m"
	cDim      = "\033[2m"
	cItalic   = "\033[3m"
	cIcon     = "\033[36m"    // cyan for icons
	cLabel    = "\033[1m"     // bold for labels
	cValue    = "\033[37m"    // white (light gray) for values
	cAccent   = "\033[96m"    // bright cyan accent
	cTitle    = "\033[1;96m"  // bold bright cyan for titles
	cSep      = "\033[90m"    // bright black (dark gray) separator
	cBarLow   = "\033[32m"    // green
	cBarMid   = "\033[33m"    // yellow
	cBarHigh  = "\033[31m"    // red
	cBarBg    = "\033[90m"    // bright black (dark gray) bar background
	cGraphLo  = "\033[32m"    // green
	cGraphMd  = "\033[33m"    // yellow
	cGraphHi  = "\033[31m"    // red
	cDot      = "\033[96m"    // bright cyan active dot
	cDotDim   = "\033[90m"    // bright black (dark gray) inactive dot
	cArrow    = "\033[1;96m"  // bold bright cyan
	cArrowDim = "\033[90m"    // bright black (dark gray)
)

// ── Navigation chrome ──────────────────────────────────────────────

func (ss *SlideShow) RenderCurrent(width, height int) []string {
	ss.mu.RLock()
	idx := ss.current
	slide := ss.slides[idx]
	total := len(ss.slides)
	ss.mu.RUnlock()

	if width < 10 || height < 3 {
		return nil
	}

	// Nav: ◂  Icon Title  ● ○ ○  ▸
	leftArrow := cArrow + "◂" + cReset
	rightArrow := cArrow + "▸" + cReset
	if idx == 0 {
		leftArrow = cArrowDim + "◂" + cReset
	}
	if idx == total-1 {
		rightArrow = cArrowDim + "▸" + cReset
	}

	icon := slide.Icon()
	title := slide.Title()
	dots := ss.renderDots(idx, total)

	navInner := fmt.Sprintf(" %s  %s%s %s%s  %s ", leftArrow, cIcon, icon, cTitle, title, dots)
	navInner += " " + rightArrow + " "
	navInnerW := 1 + 1 + 2 + visualLen(icon) + 1 + visualLen(title) + 2 + dotsWidth(total) + 1 + 1 + 1 + 1

	padding := (width - navInnerW) / 2
	if padding < 0 {
		padding = 0
	}

	navLine := strings.Repeat(" ", padding) + navInner

	// Thin separator
	sepLine := cSep + strings.Repeat("─", width) + cReset

	// Content
	contentHeight := height - 2
	if contentHeight < 1 {
		contentHeight = 1
	}
	contentLines := slide.Render(width, contentHeight)

	lines := make([]string, 0, len(contentLines)+2)
	lines = append(lines, navLine)
	lines = append(lines, sepLine)
	lines = append(lines, contentLines...)
	// No padding — let composeFrame control the total height
	if len(lines) > height {
		lines = lines[:height]
	}
	return lines
}

func (ss *SlideShow) renderDots(current, total int) string {
	var b strings.Builder
	for i := 0; i < total; i++ {
		if i == current {
			b.WriteString(cDot + "●" + cReset)
		} else {
			b.WriteString(cDotDim + "○" + cReset)
		}
		if i < total-1 {
			b.WriteByte(' ')
		}
	}
	return b.String()
}

func dotsWidth(total int) int {
	if total <= 0 {
		return 0
	}
	return total + (total - 1) // dots + spaces
}

func (ss *SlideShow) NavClickRegion(width int) (int, int, int, int) {
	ss.mu.RLock()
	idx := ss.current
	slide := ss.slides[idx]
	total := len(ss.slides)
	ss.mu.RUnlock()

	icon := slide.Icon()
	title := slide.Title()
	navInnerW := 1 + 1 + 2 + visualLen(icon) + 1 + visualLen(title) + 2 + dotsWidth(total) + 1 + 1 + 1 + 1
	padding := (width - navInnerW) / 2
	if padding < 0 {
		padding = 0
	}

	leftStart := padding + 1
	leftEnd := padding + 3
	rightStart := padding + navInnerW - 2
	rightEnd := padding + navInnerW

	return leftStart, leftEnd, rightStart, rightEnd
}

func (ss *SlideShow) RefreshLiveSlides() {
	ss.mu.RLock()
	slide := ss.slides[ss.current]
	ss.mu.RUnlock()
	if slide.NeedsRefresh() {
		slide.Refresh()
	}
}

func (ss *SlideShow) StartRefreshLoop(interval time.Duration, stop chan struct{}) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				ss.RefreshLiveSlides()
			}
		}
	}()
}

// ── Shared rendering primitives ────────────────────────────────────

const iconW = 2 // all icons occupy 2 visible columns (nerd font glyph + space)

// renderRow renders: " icon Label···  value" with label padded to labelW.
// Total visible prefix width is: 1 + iconW + labelW + 2 = labelW + 5
func renderRow(icon, label, value string, labelW, width int) string {
	if value == "" {
		value = "-"
	}
	paddedLabel := label + strings.Repeat(" ", max(0, labelW-visualLen(label)))
	return fmt.Sprintf(" %s%s%s %s%s%s  %s%s%s",
		cIcon, icon, cReset,
		cLabel, paddedLabel, cReset,
		cValue, value, cReset)
}

// renderRowFixed is like renderRow but pads the entire output to exactly totalW visible chars.
// Used for multi-column layouts so columns align.
func renderRowFixed(icon, label, value string, labelW, totalW int) string {
	if value == "" {
		value = "-"
	}
	paddedLabel := label + strings.Repeat(" ", max(0, labelW-visualLen(label)))
	raw := fmt.Sprintf(" %s%s%s %s%s%s  %s%s%s",
		cIcon, icon, cReset,
		cLabel, paddedLabel, cReset,
		cValue, value, cReset)
	vl := visualLen(raw)
	if vl < totalW {
		return raw + strings.Repeat(" ", totalW-vl)
	}
	return raw
}

// renderUsageBar renders: " icon Label···  [████░░░░]  42.1%"
func renderUsageBar(icon, label string, usage float64, labelW, width int) string {
	paddedLabel := label + strings.Repeat(" ", max(0, labelW-visualLen(label)))
	pct := fmt.Sprintf("%5.1f%%", usage*100)

	// prefix: 1 + iconW + labelW + 2 = labelW + 5
	prefixW := labelW + 5
	barW := width - prefixW - len(pct) - 1
	if barW < 8 {
		barW = 8
	}

	bar := makeBar(usage, barW)

	return fmt.Sprintf(" %s%s%s %s%s%s  %s %s",
		cIcon, icon, cReset,
		cLabel, paddedLabel, cReset,
		bar, pct)
}

// makeBar creates a colored bar with smooth partial-block fill.
func makeBar(value float64, width int) string {
	if value < 0 {
		value = 0
	}
	if value > 1 {
		value = 1
	}

	// Choose color based on value
	color := cBarLow
	if value > 0.7 {
		color = cBarMid
	}
	if value > 0.9 {
		color = cBarHigh
	}

	// Smooth fill using eighths: ▏▎▍▌▋▊▉█
	eighths := []rune{' ', '▏', '▎', '▍', '▌', '▋', '▊', '▉', '█'}
	totalEighths := int(value * float64(width) * 8)
	fullBlocks := totalEighths / 8
	partialIdx := totalEighths % 8

	var b strings.Builder
	b.WriteString(color)
	for i := 0; i < fullBlocks && i < width; i++ {
		b.WriteRune('█')
	}
	if fullBlocks < width && partialIdx > 0 {
		b.WriteRune(eighths[partialIdx])
		fullBlocks++
	}
	b.WriteString(cReset)
	b.WriteString(cBarBg)
	remaining := width - fullBlocks
	if remaining < 0 {
		remaining = 0
	}
	for i := 0; i < remaining; i++ {
		b.WriteRune('░')
	}
	b.WriteString(cReset)
	return b.String()
}

// renderSparkGraph renders a compact block-character graph with color gradient.
// height should be small (2-3 rows) to stay compact.
func renderSparkGraph(values []float64, width, height int) []string {
	if width < 1 || height < 1 {
		return nil
	}

	blocks := []rune{' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

	data := values
	if len(data) > width {
		data = data[len(data)-width:]
	}

	lines := make([]string, height)
	for row := 0; row < height; row++ {
		var b strings.Builder
		b.WriteString("  ")
		threshold := 1.0 - float64(row+1)/float64(height)
		nextThreshold := 1.0 - float64(row)/float64(height)
		step := nextThreshold - threshold

		for _, v := range data {
			// Color based on value
			color := cGraphLo
			if v > 0.7 {
				color = cGraphMd
			}
			if v > 0.9 {
				color = cGraphHi
			}

			if v <= threshold {
				b.WriteByte(' ')
			} else if v >= nextThreshold {
				b.WriteString(color)
				b.WriteRune('█')
				b.WriteString(cReset)
			} else {
				frac := (v - threshold) / step
				idx := int(frac * float64(len(blocks)-1))
				if idx >= len(blocks) {
					idx = len(blocks) - 1
				}
				b.WriteString(color)
				b.WriteRune(blocks[idx])
				b.WriteString(cReset)
			}
		}
		lines[row] = b.String()
	}

	return lines
}

// visualLen returns the visible character width of an ANSI string.
func visualLen(s string) int {
	w := 0
	inEsc := false
	for _, r := range s {
		if r == '\033' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		w++
	}
	return w
}

func formatBytes(bytes uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)
	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.1f TiB", float64(bytes)/float64(TB))
	case bytes >= GB:
		return fmt.Sprintf("%.1f GiB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MiB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KiB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// renderColorPalette renders the classic terminal color strip.
func renderColorPalette() string {
	var b strings.Builder
	b.WriteString("  ")
	for i := 0; i < 8; i++ {
		b.WriteString(fmt.Sprintf("\033[48;5;%dm   ", i))
	}
	b.WriteString(cReset + "\n  ")
	for i := 8; i < 16; i++ {
		b.WriteString(fmt.Sprintf("\033[48;5;%dm   ", i))
	}
	b.WriteString(cReset)
	return b.String()
}
