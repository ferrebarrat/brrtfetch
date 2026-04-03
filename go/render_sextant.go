package main

import (
	"bytes"
	"fmt"
	"image"
)

type SextantRenderer struct{}

func (sr *SextantRenderer) Render(buf *bytes.Buffer, img *image.RGBA, cfg Config) {
	bounds := img.Bounds()
	imgW, imgH := bounds.Dx(), bounds.Dy()

	dotW, dotH := cfg.Width*2, cfg.Height*3
	scaleX := float64(imgW) / float64(dotW)
	scaleY := float64(imgH) / float64(dotH)

	for y := 0; y < cfg.Height; y++ {
		buf.WriteString("\x1b[0m") 

		for x := 0; x < cfg.Width; x++ {
			var rSum, gSum, bSum, aSum uint64
			var count uint64
			mask := 0

			for sy := 0; sy < 3; sy++ {
				for sx := 0; sx < 2; sx++ {
					dotX := int(float64(x*2+sx) * scaleX)
					dotY := int(float64(y*3+sy) * scaleY)

					if dotX < imgW && dotY < imgH {
						off := dotY*img.Stride + dotX*4
						r, g, b, a := img.Pix[off], img.Pix[off+1], img.Pix[off+2], img.Pix[off+3]
						
						lum := 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
						
						// --- THE FIX: ALPHA-AWARE THRESHOLDING ---
						// If the GIF says this part isn't transparent (a > 20), 
						// we want to see it even if it's quite dark.
						// We use a much lower threshold (30 instead of 110).
						threshold := 1.0 / cfg.Multiplier
						if a > 40 && lum > threshold {
							mask |= (1 << uint(sy*2+sx))
						}

						rSum += uint64(r)
						gSum += uint64(g)
						bSum += uint64(b)
						aSum += uint64(a)
						count++
					}
				}
			}

			// If any sub-pixels are active, render the character.
			// Also ensure the overall cell isn't mostly transparent.
			if count > 0 && mask > 0 && (aSum/count) > 30 {
				char := sr.getSextantChar(mask)
				if cfg.Color {
					fmt.Fprintf(buf, "\x1b[38;2;%d;%d;%dm%s", rSum/count, gSum/count, bSum/count, char)
				} else {
					buf.WriteString(char)
				}
			} else {
				buf.WriteByte(' ')
			}
		}
		buf.WriteString("\x1b[0m")
		if y < cfg.Height-1 {
			buf.WriteByte('\n')
		}
	}
}

func (sr *SextantRenderer) getSextantChar(mask int) string {
	if mask == 0 { return " " }
	if mask == 0x3F { return "█" }
	
	// Sextant characters are U+1FB00 + (mask - 1)
	r := rune(0x1FB00 + (mask - 1))
	return string(r)
}
