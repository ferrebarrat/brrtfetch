package main

import (
	"bytes"
	"fmt"
	"image"
	"math"
)

type CRTRenderer struct{}

func (cr *CRTRenderer) Render(buf *bytes.Buffer, img *image.RGBA, cfg Config) {
	bounds := img.Bounds()
	imgW, imgH := bounds.Dx(), bounds.Dy()

	// We use the same vertical logic as half-blocks, 
	// but we only render the top half of the cell to create the scanline gap.
	scaleX := float64(imgW) / float64(cfg.Width)
	scaleY := float64(imgH) / float64(cfg.Height)

	const SCANLINE_CHAR = "▆" // A thick-ish mid-bar for that retro look

	for y := 0; y < cfg.Height; y++ {
		// Reset and apply a subtle "glow" using Bold
		buf.WriteString("\x1b[0m\x1b[1m")

		for x := 0; x < cfg.Width; x++ {
			px := int(float64(x) * scaleX)
			py := int(float64(y) * scaleY)
			
			off := py*img.Stride + px*4
			if off >= len(img.Pix)-3 {
				buf.WriteByte(' ')
				continue
			}

			r, g, b, a := img.Pix[off], img.Pix[off+1], img.Pix[off+2], img.Pix[off+3]

			// Alpha threshold
			if a < 128 {
				buf.WriteByte(' ')
				continue
			}

			// Boost brightness for the 'phosphor' effect
			// We push the colors toward their max to simulate a glowing screen
			br := float64(r) * cfg.Multiplier * 1.2
			bg := float64(g) * cfg.Multiplier * 1.2
			bb := float64(b) * cfg.Multiplier * 1.2

			if cfg.Color {
				// We use a specific background-foreground combo to make the 
				// scanline look like it's floating.
				fmt.Fprintf(buf, "\x1b[38;2;%d;%d;%dm%s", 
					uint8(math.Min(255, br)), 
					uint8(math.Min(255, bg)), 
					uint8(math.Min(255, bb)), 
					SCANLINE_CHAR)
			} else {
				lum := 0.2126*br + 0.7152*bg + 0.0722*bb
				if lum > 128 {
					buf.WriteString(SCANLINE_CHAR)
				} else {
					buf.WriteByte(' ')
				}
			}
		}
		// Reset formatting and newline
		buf.WriteString("\x1b[0m\n")
	}
}
