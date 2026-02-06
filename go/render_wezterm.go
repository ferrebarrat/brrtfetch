package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
)

type WeztermRenderer struct{}

func (w *WeztermRenderer) Render(buf *bytes.Buffer, img *image.RGBA, cfg Config) {
	// 1. Encode to PNG
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		buf.WriteString("[WezTerm Encode Error]")
		return
	}

	b64Data := base64.StdEncoding.EncodeToString(pngBuf.Bytes())

	// 2. Setup Cursor for Side-by-Side Layout
	// We save the cursor position so we can return to the top-right
	// for the sysinfo after the image is "printed."
	buf.WriteString("\x1b[s")

	// 3. WezTerm / iTerm2 Hybrid Sequence
	// We use 'doNotMoveCursor=1', a WezTerm-specific extension that prevents 
	// the terminal from advancing the cursor to the bottom of the image.
	fmt.Fprintf(buf, "\x1b]1337;File=inline=1;width=%d;height=%d;doNotMoveCursor=1:%s\a",
		cfg.Width, cfg.Height, b64Data)

	// 4. Position for Sysinfo
	// Restore cursor to top-left of image, then move right by the image width + padding
	targetColumn := cfg.Width + 5
	buf.WriteString(fmt.Sprintf("\x1b[u\x1b[%dG", targetColumn))
}
