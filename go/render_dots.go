package main

import (
	"bytes"
	"fmt"
	"image"
)

type DotRenderer struct{}

func (d *DotRenderer) Render(buf *bytes.Buffer, img *image.RGBA, cfg Config) {
	// Standard Bullet (U+2022) is the most compatible 'big' dot.
	// It's almost always single-width, preventing the 'broken layout' issue.
	const DOT = "⬤" 

	imgBounds := img.Bounds()
	imgW := imgBounds.Dx()
	imgH := imgBounds.Dy()

	for y := 0; y < cfg.Height; y++ {
		// Start each line by ensuring color is reset
		buf.WriteString("\x1b[0m")

		for x := 0; x < cfg.Width; x++ {
			// Precise pixel mapping to avoid 'shimmering' on resize
			px := (x * imgW) / cfg.Width
			py := (y * imgH) / cfg.Height
			
			offset := py*img.Stride + px*4
			if offset < 0 || offset+3 >= len(img.Pix) {
				buf.WriteByte(' ')
				continue
			}

			r, g, b, a := img.Pix[offset], img.Pix[offset+1], img.Pix[offset+2], img.Pix[offset+3]

			// 1. Alpha Check (The 'Ghosting' Fix)
			// If it's transparent, we MUST write a space to maintain the grid.
			if a < 128 {
				buf.WriteByte(' ')
				continue
			}

			if cfg.Color {
				// 2. The 'No-Bleed' Color implementation
				// \x1b[38;2;R;G;Bm -> Set FG color
				// %s -> The Dot
				// \x1b[0m -> RESET IMMEDIATELY so the next cell is clean
				fmt.Fprintf(buf, "\x1b[38;2;%d;%d;%dm%s", r, g, b, DOT)
			} else {
				// Grayscale logic
				lum := 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
				if lum > (100 * cfg.Multiplier) {
					buf.WriteString(DOT)
				} else {
					buf.WriteByte(' ')
				}
			}
		}

		// 3. The 'Sysinfo' Fix:
		// Reset color and ensure we don't have trailing garbage before the newline
		buf.WriteString("\x1b[0m")
		if y < cfg.Height-1 {
			buf.WriteByte('\n')
		}
	}
}
