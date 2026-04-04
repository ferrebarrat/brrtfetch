package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
)

type KittyRenderer struct{}

func (k *KittyRenderer) Render(buf *bytes.Buffer, img *image.RGBA, cfg Config) {
<<<<<<< HEAD
	// Save cursor so we can return to the top after the image is placed
	buf.WriteString("\x1b[s")

	// 1. Clear previous image ID 1
=======
	// Save cursor, move to top-left for image placement
	buf.WriteString("\x1b[s\x1b[1;1H")

	// Delete previous image ID 1
>>>>>>> bar-test
	buf.WriteString("\x1b_Ga=d,d=i,i=1\x1b\\")

	// Encode to PNG
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		buf.WriteString("\x1b[u") // restore cursor even on error
		return
	}

	b64Data := base64.StdEncoding.EncodeToString(pngBuf.Bytes())

<<<<<<< HEAD
	// 3. Chunk the Base64 data to prevent terminal crashes
	// We send 4096 bytes at a time (standard safe chunk size)
=======
	// Chunk the base64 data (4096 bytes per chunk)
>>>>>>> bar-test
	const chunkSize = 4096
	totalLen := len(b64Data)

	for i := 0; i < totalLen; i += chunkSize {
		end := i + chunkSize
		hasMore := 1
		if end >= totalLen {
			end = totalLen
			hasMore = 0
		}

		if i == 0 {
			// First chunk: C=0 means don't move cursor after display
			fmt.Fprintf(buf, "\x1b_Ga=T,f=100,t=d,i=1,C=0,c=%d,r=%d,m=%d;", cfg.Width, cfg.Height, hasMore)
		} else {
			fmt.Fprintf(buf, "\x1b_Gm=%d;", hasMore)
		}

		buf.WriteString(b64Data[i:end])
		buf.WriteString("\x1b\\")
	}

<<<<<<< HEAD
	// 4. Restore cursor to top of image, move right past image area
	buf.WriteString("\x1b[u")
	fmt.Fprintf(buf, "\x1b[%dC", cfg.Width)

	// 5. Output cfg.Height lines so composeFrame can place sysinfo alongside
	for y := 1; y < cfg.Height; y++ {
		buf.WriteByte('\n')
		fmt.Fprintf(buf, "\x1b[%dC", cfg.Width)
	}
=======
	// Restore cursor to original position
	buf.WriteString("\x1b[u")
>>>>>>> bar-test
}
