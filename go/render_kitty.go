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
	// 1. Clear previous image ID 1
	buf.WriteString("\x1b_Ga=d,d=i,i=1\x1b\\")

	// 2. Encode to PNG
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		return
	}

	b64Data := base64.StdEncoding.EncodeToString(pngBuf.Bytes())
	
	// 3. Chunk the Base64 data to prevent terminal crashes
	// We send 4096 bytes at a time (standard safe chunk size)
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
			// First chunk: transmit headers (a=T, f=100, i=1, C=1, etc.)
			fmt.Fprintf(buf, "\x1b_Ga=T,f=100,t=d,i=1,C=1,c=%d,r=%d,m=%d;", cfg.Width, cfg.Height, hasMore)
		} else {
			// Subsequent chunks: only need the 'm' flag and the data
			fmt.Fprintf(buf, "\x1b_Gm=%d;", hasMore)
		}
		
		buf.WriteString(b64Data[i:end])
		buf.WriteString("\x1b\\")
	}

	// 4. Position cursor for sysinfo next to the image
	fmt.Fprintf(buf, "\x1b[%dC", cfg.Width+2)
}
