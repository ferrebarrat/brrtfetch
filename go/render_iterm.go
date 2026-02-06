package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
)

type ITermRenderer struct{}

func (i *ITermRenderer) Render(buf *bytes.Buffer, img *image.RGBA, cfg Config) {
	// 1. Encode the frame to PNG
	// iTerm2 handles PNG/JPG best for the inline protocol
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		buf.WriteString("[iTerm Encode Error]")
		return
	}

	b64Data := base64.StdEncoding.EncodeToString(pngBuf.Bytes())

	// 2. THE ATOMIC FIX (Cursor Management)
	// Save cursor position so we can return to the top-right after rendering
	buf.WriteString("\x1b[s")

	// 3. iTerm2 Image Sequence
	// File=inline=1 : Tells iTerm to display it immediately
	// width/height  : Set to cfg dimensions (in character cells)
	// :             : Separator before base64 data
	// \a            : The BEL character (ends the sequence)
	fmt.Fprintf(buf, "\x1b]1337;File=inline=1;width=%d;height=%d;preserveAspectRatio=1:%s\a",
		cfg.Width, cfg.Height, b64Data)

	// 4. Position cursor for side-by-side info
	// Restore cursor to the top of where the image started
	// Then move right by the width of the image plus a small padding
	targetColumn := cfg.Width + 4
	buf.WriteString(fmt.Sprintf("\x1b[u\x1b[%dG", targetColumn))
}
