package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"
	"golang.org/x/term"
)

const maxRawHistory = 2 << 20 // 2MB of PTY output history for reflow on resize

type ShellSession struct {
	cmd          *exec.Cmd
	ptyMaster    *os.File
	oldState     *term.State
	barHeight    int
	mu           sync.RWMutex
	done         chan struct{}
	vte          *vt.Emulator
	scrollOffset int
	dirty        chan struct{}
	termW, termH int
	prevLines    []string    // for differential rendering
	rawHistory   []byte      // raw PTY output for VTE rebuild on resize
	slideShow    *SlideShow  // optional: for click-to-navigate slides
	infoXOffset  int         // x offset where info area starts (gifWidth + 3)
}

func NewShellSession(barHeight, termW, termH int) (*ShellSession, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	shellRows := termH - barHeight
	if shellRows < 2 {
		shellRows = 2
	}

	cmd := exec.Command(shell)
	cmd.Env = os.Environ()

	master, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("pty.Start: %w", err)
	}

	_ = pty.Setsize(master, &pty.Winsize{
		Rows: uint16(shellRows),
		Cols: uint16(termW),
	})

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		master.Close()
		cmd.Process.Kill()
		return nil, fmt.Errorf("term.MakeRaw: %w", err)
	}

	emulator := vt.NewEmulator(termW, shellRows)
	emulator.SetScrollbackSize(10000)

	return &ShellSession{
		cmd:       cmd,
		ptyMaster: master,
		oldState:  oldState,
		barHeight: barHeight,
		done:      make(chan struct{}),
		vte:       emulator,
		dirty:     make(chan struct{}, 1),
		termW:     termW,
		termH:     termH,
	}, nil
}

func (s *ShellSession) Run() {
	// Enable mouse tracking for scroll wheel
	os.Stdout.WriteString(ANSI_MOUSE_ON)

	// Stdin parser: keyboard → PTY, wheel → scroll
	go s.readStdin()

	// PTY output → VTE
	go s.readPTY()

	// VTE responses (DA etc) → PTY
	go s.readVTEResponses(s.vte)

	// Wait for shell exit
	go func() {
		s.cmd.Wait()
		close(s.done)
	}()
}

func (s *ShellSession) readPTY() {
	buf := make([]byte, 32768)
	for {
		n, err := s.ptyMaster.Read(buf)
		if n > 0 {
			s.mu.Lock()
			s.vte.Write(buf[:n])
			// Store raw output for reflow on resize
			s.rawHistory = append(s.rawHistory, buf[:n]...)
			if len(s.rawHistory) > maxRawHistory {
				s.rawHistory = s.rawHistory[len(s.rawHistory)-maxRawHistory:]
			}
			s.mu.Unlock()
			// Signal dirty (non-blocking)
			select {
			case s.dirty <- struct{}{}:
			default:
			}
		}
		if err != nil {
			return
		}
	}
}

func (s *ShellSession) readVTEResponses(emulator *vt.Emulator) {
	buf := make([]byte, 4096)
	for {
		n, err := emulator.Read(buf)
		if n > 0 {
			s.ptyMaster.Write(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

func (s *ShellSession) readStdin() {
	buf := make([]byte, 4096)
	var pending []byte
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			var input []byte
			if len(pending) > 0 {
				input = append(pending, buf[:n]...)
				pending = nil
			} else {
				input = buf[:n]
			}
			remainder := s.processInput(input)
			if len(remainder) > 0 {
				pending = make([]byte, len(remainder))
				copy(pending, remainder)
			}
		}
		if err != nil {
			if len(pending) > 0 {
				s.ptyMaster.Write(pending)
			}
			return
		}
	}
}

func (s *ShellSession) processInput(data []byte) []byte {
	i := 0
	for i < len(data) {
		// Try to match SGR mouse sequence: \033[< ... M or m
		if data[i] == 0x1b {
			// Check if we have enough bytes to determine sequence type
			if i+1 >= len(data) {
				// Incomplete escape at end of buffer - return as remainder
				return data[i:]
			}
			if i+2 < len(data) && data[i+1] == '[' && data[i+2] == '<' {
				// SGR mouse sequence: find terminator M or m
				end := i + 3
				for end < len(data) && data[end] != 'M' && data[end] != 'm' {
					// Validate it's a valid mouse sequence char (digits, semicolons)
					if !((data[end] >= '0' && data[end] <= '9') || data[end] == ';') {
						break
					}
					end++
				}
				if end >= len(data) {
					// Incomplete mouse sequence - return as remainder
					return data[i:]
				}
				if data[end] == 'M' || data[end] == 'm' {
					// Complete mouse sequence - parse it
					seq := string(data[i+3 : end])
					parts := strings.Split(seq, ";")
					if len(parts) == 3 {
						btn, _ := strconv.Atoi(parts[0])
						mx, _ := strconv.Atoi(parts[1])
						my, _ := strconv.Atoi(parts[2])
						s.handleMouse(btn, mx, my)
					}
					i = end + 1
					continue
				}
				// Not a valid mouse sequence, fall through to forward as keyboard
			}

			// APC sequence (\x1b_ ... ST): kitty graphics responses etc — consume silently
			if i+1 < len(data) && data[i+1] == '_' {
				end := i + 2
				for end < len(data) {
					// String Terminator is \x1b\\ or \x07
					if data[end] == 0x07 {
						i = end + 1
						break
					}
					if end+1 < len(data) && data[end] == 0x1b && data[end+1] == '\\' {
						i = end + 2
						break
					}
					end++
				}
				if end >= len(data) {
					// Incomplete APC — buffer remainder
					return data[i:]
				}
				continue
			}

			// Non-mouse escape sequence: snap to live and forward to PTY
			s.snapToLive()
			// Find end of this escape sequence
			start := i
			i++ // skip ESC
			if i < len(data) && data[i] == '[' {
				// CSI sequence
				i++
				for i < len(data) && data[i] >= 0x20 && data[i] <= 0x3F {
					i++
				}
				if i < len(data) {
					i++ // final byte
				} else {
					return data[start:] // incomplete
				}
			} else if i < len(data) {
				i++ // 2-byte escape
			}
			s.ptyMaster.Write(data[start:i])
			continue
		}

		// Regular character(s): snap to live, forward to PTY
		s.snapToLive()
		start := i
		i++
		for i < len(data) && data[i] != 0x1b {
			i++
		}
		s.ptyMaster.Write(data[start:i])
	}
	return nil
}

func (s *ShellSession) snapToLive() {
	s.mu.Lock()
	if s.scrollOffset > 0 {
		s.scrollOffset = 0
		// Signal redraw needed
		select {
		case s.dirty <- struct{}{}:
		default:
		}
	}
	s.mu.Unlock()
}

func (s *ShellSession) handleMouse(btn, x, y int) {
	if btn == 64 { // wheel up
		s.mu.Lock()
		maxScroll := s.vte.ScrollbackLen()
		s.scrollOffset += 3
		if s.scrollOffset > maxScroll {
			s.scrollOffset = maxScroll
		}
		s.mu.Unlock()
		select {
		case s.dirty <- struct{}{}:
		default:
		}
		return
	}
	if btn == 65 { // wheel down
		s.mu.Lock()
		s.scrollOffset -= 3
		if s.scrollOffset < 0 {
			s.scrollOffset = 0
		}
		s.mu.Unlock()
		select {
		case s.dirty <- struct{}{}:
		default:
		}
		return
	}
	// Left click (btn=0) in bar area: check slideshow arrow clicks
	if btn == 0 && s.slideShow != nil {
		s.mu.RLock()
		bh := s.barHeight
		xOff := s.infoXOffset
		tw := s.termW
		s.mu.RUnlock()
		if y <= bh {
			infoW := tw - xOff
			if infoW < 10 { infoW = 10 }
			ls, le, rs, re := s.slideShow.NavClickRegion(infoW)
			// Adjust for info area offset
			infoX := x - xOff
			if infoX >= ls && infoX <= le {
				s.slideShow.Prev()
			} else if infoX >= rs && infoX <= re {
				s.slideShow.Next()
			}
		}
	}
	// All other mouse events: consumed (not forwarded to PTY)
}

func (s *ShellSession) RenderShellRegion(writer *bufio.Writer) {
	s.mu.RLock()
	scrollOffset := s.scrollOffset
	shellRows := s.termH - s.barHeight
	vteW := s.vte.Width()
	barHeight := s.barHeight

	if s.vte.IsAltScreen() {
		scrollOffset = 0
	}

	if scrollOffset == 0 {
		// Live mode: render VTE screen
		rendered := s.vte.Render()
		pos := s.vte.CursorPosition()
		s.mu.RUnlock()

		lines := strings.Split(rendered, "\n")
		changed := false
		newPrev := make([]string, shellRows)

		for row := 0; row < shellRows; row++ {
			var line string
			if row < len(lines) {
				line = lines[row]
			}
			newPrev[row] = line
			if row < len(s.prevLines) && s.prevLines[row] == line {
				continue
			}
			changed = true
			writer.WriteString(MoveCursor(barHeight+1+row, 1))
			writer.WriteString(line)
			writer.WriteString("\033[0m" + ANSI_CLEAR_LINE)
		}
		// Clear any extra rows from previous render
		for row := shellRows; row < len(s.prevLines); row++ {
			writer.WriteString(MoveCursor(barHeight+1+row, 1))
			writer.WriteString(ANSI_CLEAR_LINE)
		}
		s.prevLines = newPrev

		if changed || true { // always position cursor
			writer.WriteString(MoveCursor(barHeight+1+pos.Y, pos.X+1))
			writer.WriteString(ANSI_SHOW_CURSOR)
		}
	} else {
		// Scrollback mode
		sbLen := s.vte.ScrollbackLen()
		vteH := s.vte.Height()
		sb := s.vte.Scrollback()

		newPrev := make([]string, shellRows)

		for row := 0; row < shellRows; row++ {
			lineIdx := sbLen - scrollOffset + row
			writer.WriteString(MoveCursor(barHeight+1+row, 1))

			if lineIdx < 0 {
				newPrev[row] = ""
				writer.WriteString(ANSI_CLEAR_LINE)
			} else if lineIdx < sbLen {
				// Render from scrollback
				line := sb.Line(lineIdx)
				rendered := line.Render()
				newPrev[row] = rendered
				writer.WriteString(rendered)
				writer.WriteString("\033[0m" + ANSI_CLEAR_LINE)
			} else {
				// On-screen row
				screenRow := lineIdx - sbLen
				if screenRow < vteH {
					// Render screen row cell by cell
					var lineStr strings.Builder
					for x := 0; x < vteW; x++ {
						cell := s.vte.CellAt(x, screenRow)
						if cell != nil && cell.Content != "" {
							lineStr.WriteString(cell.Style.Styled(cell.Content))
						} else {
							lineStr.WriteByte(' ')
						}
					}
					rendered := lineStr.String()
					newPrev[row] = rendered
					writer.WriteString(rendered)
					writer.WriteString("\033[0m" + ANSI_CLEAR_LINE)
				} else {
					newPrev[row] = ""
					writer.WriteString(ANSI_CLEAR_LINE)
				}
			}
		}
		s.prevLines = newPrev
		s.mu.RUnlock()

		// Show scroll indicator
		indicator := fmt.Sprintf(" [%d/%d] ", scrollOffset, sbLen)
		col := s.termW - len(indicator) + 1
		if col < 1 {
			col = 1
		}
		writer.WriteString(MoveCursor(s.termH, col))
		writer.WriteString("\033[7m") // reverse video
		writer.WriteString(indicator)
		writer.WriteString("\033[0m")
		writer.WriteString(ANSI_HIDE_CURSOR)
	}
}

func (s *ShellSession) Resize(termW, termH, newBarHeight int) {
	shellRows := termH - newBarHeight
	if shellRows < 2 {
		shellRows = 2
	}

	s.mu.Lock()
	oldVTE := s.vte

	// Create fresh VTE at new dimensions and replay history for reflow
	newVTE := vt.NewEmulator(termW, shellRows)
	newVTE.SetScrollbackSize(10000)
	if len(s.rawHistory) > 0 {
		newVTE.Write(s.rawHistory)
	}

	s.vte = newVTE
	s.barHeight = newBarHeight
	s.termW = termW
	s.termH = termH
	s.scrollOffset = 0
	s.prevLines = nil
	s.mu.Unlock()

	// Close old VTE (causes old readVTEResponses goroutine to exit)
	oldVTE.Close()

	// Start new VTE response reader for the new emulator
	go s.readVTEResponses(newVTE)

	_ = pty.Setsize(s.ptyMaster, &pty.Winsize{
		Rows: uint16(shellRows),
		Cols: uint16(termW),
	})
	s.cmd.Process.Signal(syscall.SIGWINCH)
}

func (s *ShellSession) Cleanup() {
	// Disable mouse tracking
	os.Stdout.WriteString(ANSI_MOUSE_OFF)

	// Close VTE
	s.vte.Close()

	// Reset scroll region and clear bar area
	os.Stdout.WriteString(ANSI_RESET_SCROLL)
	s.mu.Lock()
	for i := 0; i < s.barHeight; i++ {
		os.Stdout.WriteString(MoveCursor(1+i, 1) + ANSI_CLEAR_LINE)
	}
	os.Stdout.WriteString(MoveCursor(s.barHeight+1, 1))
	os.Stdout.WriteString(ANSI_SHOW_CURSOR + ANSI_ENABLE_WRAP)
	s.mu.Unlock()

	if s.oldState != nil {
		term.Restore(int(os.Stdin.Fd()), s.oldState)
	}
	s.ptyMaster.Close()
}

