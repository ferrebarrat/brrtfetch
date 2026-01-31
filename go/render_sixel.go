package main

import (
	"bytes"
	"fmt"
	"image"

	"github.com/mattn/go-sixel"
)

type SixelRenderer struct{}

func (s *SixelRenderer) Render(buf *bytes.Buffer, img *image.RGBA, cfg Config) {
	// 1. Terminal Setup
	// \x1b[?80l  - Disable Sixel scrolling (prevents image from jumping)
	// \x1b[?1070h - Enable Sixel transparency (essential for non-black background)
	buf.WriteString("\x1b[?80l\x1b[?1070h")

	// 2. Save Cursor Position
	buf.WriteString("\x1b[s")

	// 3. Encode the Sixel data
	// We use a temporary buffer to ensure the escape sequences are clean
	var sBuf bytes.Buffer
	enc := sixel.NewEncoder(&sBuf)
	enc.Dither = true
	// Many encoders fail transparency if the palette is too large. 
	// go-sixel handles this internally, but we must ensure we encode the RGBA directly.
	if err := enc.Encode(img); err != nil {
		return
	}
	buf.Write(sBuf.Bytes())

	// 4. The "Hard Reset" for the cursor
	// \x1b[u - Restore cursor to top-left
	// \x1b[H - Some terminals need a Home-relative jump if [u] is ignored
	// Since you want it side-by-side, we restore, then nudge.
	buf.WriteString("\x1b[u")

	// 5. Move right to bypass the image's "hitbox"
	// We use \x1b[s/u but also a Carriage Return (\r) + Forward to be safe.
	fmt.Fprintf(buf, "\r\x1b[%dC", cfg.Width+2)
}
