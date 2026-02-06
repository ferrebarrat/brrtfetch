package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os/exec"
	"strconv"
	"strings"
)

type ChafaRenderer struct{}

func (c *ChafaRenderer) Render(buf *bytes.Buffer, img *image.RGBA, cfg Config) {
	// 1. Get image symbols from Chafa
	args := []string{
		"--size", strconv.Itoa(cfg.Width) + "x" + strconv.Itoa(cfg.Height),
		"--format", "symbols",
		"-",
	}

	if !cfg.Color {
		args = append(args, "--color-mode", "bw")
	}

	cmd := exec.Command("chafa", args...)
	stdin, _ := cmd.StdinPipe()
	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf

	if err := cmd.Start(); err != nil {
		buf.WriteString("[Chafa Error]")
		return
	}

	go func() {
		defer stdin.Close()
		_ = png.Encode(stdin, img)
	}()

	_ = cmd.Wait()

	// 2. Prepare the Canvas
	chafaLines := strings.Split(strings.TrimRight(outBuf.String(), "\n\r"), "\n")
	targetColumn := cfg.Width + 6
	
	// Ensure we clear enough space for the sysinfo length even if GIF is small
	const MaxPossibleLines = 40 
	
	// 3. THE ATOMIC FIX:
	// \x1b[7  = Save Cursor Position (DECSC)
	buf.WriteString("\x1b[s")

	for i := 0; i < MaxPossibleLines; i++ {
		// Clear current line to prevent resize artifacts
		buf.WriteString("\x1b[2K")

		if i < len(chafaLines) {
			buf.WriteString(chafaLines[i])
		}

		// Move to the exact sysinfo column
		buf.WriteString(fmt.Sprintf("\x1b[%dG", targetColumn))

		// If this is not the last line of our safety block, move down
		if i < MaxPossibleLines-1 {
			buf.WriteString("\n")
		}
	}

	// 4. THE MAGIC TRICK:
	// Move the cursor back to the start of the block so that 
	// the very first line of your sysinfo appends to the top-right
	// instead of the bottom-left.
	// \x1b[u = Restore Cursor Position (DECRC)
	// \x1b[%dC = Move right to the target column
	buf.WriteString(fmt.Sprintf("\x1b[u\x1b[%dC", targetColumn-1))
}
