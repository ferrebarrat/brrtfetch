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

	// 2. Save cursor so we can return to the top after the image is placed
	buf.WriteString("\x1b[s")

	// 3. WezTerm / iTerm2 Hybrid Sequence
	// doNotMoveCursor=1 prevents the terminal from advancing the cursor
	fmt.Fprintf(buf, "\x1b]1337;File=inline=1;width=%d;height=%d;doNotMoveCursor=1:%s\a",
		cfg.Width, cfg.Height, b64Data)

	// 4. Restore cursor to top of image, move right past image area
	buf.WriteString("\x1b[u")
	fmt.Fprintf(buf, "\x1b[%dC", cfg.Width)

	// 5. Output cfg.Height lines so composeFrame can place sysinfo alongside
	for y := 1; y < cfg.Height; y++ {
		buf.WriteByte('\n')
		fmt.Fprintf(buf, "\x1b[%dC", cfg.Width)
	}
}
