package main

import (
	"bytes"
	"fmt"
	"image"
	"math"
)

type MatrixRenderer struct{}

func (mr *MatrixRenderer) Render(buf *bytes.Buffer, img *image.RGBA, cfg Config) {
	bounds := img.Bounds()
	imgW, imgH := bounds.Dx(), bounds.Dy()
	scaleX, scaleY := float64(imgW)/float64(cfg.Width), float64(imgH)/float64(cfg.Height)

	// "Hacker" set: Directional glyphs mixed with density
	// We use strings to support potential multi-byte symbols
	glyphs := []string{"0", "1", "{", "}", "[", "]", "/", "\\", "|", "-", "7", "L"}

	for y := 0; y < cfg.Height; y++ {
		buf.WriteString("\x1b[0m") // Reset per line

		for x := 0; x < cfg.Width; x++ {
			xS, yS := int(float64(x)*scaleX), int(float64(y)*scaleY)
			
			// Analyze the pixel area
			gx, gy, r, g, b, a, _ := mr.analyze(img, xS, yS)

			// 1. ULTRA-LOW THRESHOLD: If there is ANY alpha, we render.
			if a < 5 { 
				buf.WriteByte(' ')
				continue
			}

			// 2. Vector Angle Calculation
			// We use the gradient to pick the "direction" of the code
			angle := math.Atan2(gy, gx) * (180 / math.Pi)
			if angle < 0 { angle += 360 }

			// 3. Glyph Selection
			// We map the 360-degree angle to our glyph array
			glyphIdx := int((angle / 360.0) * float64(len(glyphs)))
			if glyphIdx >= len(glyphs) { glyphIdx = len(glyphs) - 1 }
			char := glyphs[glyphIdx]

			if cfg.Color {
				// We apply the multiplier directly to the color intensity
				// to ensure the "Matrix" glow is bright.
				rOut := uint8(math.Min(255, float64(r)*cfg.Multiplier))
				gOut := uint8(math.Min(255, float64(g)*cfg.Multiplier))
				bOut := uint8(math.Min(255, float64(b)*cfg.Multiplier))
				
				fmt.Fprintf(buf, "\x1b[38;2;%d;%d;%dm%s", rOut, gOut, bOut, char)
			} else {
				buf.WriteString(char)
			}
		}
		buf.WriteString("\x1b[0m\n")
	}
}

func (mr *MatrixRenderer) analyze(img *image.RGBA, x, y int) (gx, gy float64, r, g, b, a uint8, lumNorm float64) {
	var rS, gS, bS, aS, lS uint64
	var count uint64

	// 3x3 kernel for Sobel and Color averaging
	for ky := -1; ky <= 1; ky++ {
		for kx := -1; kx <= 1; kx++ {
			px, py := x+kx, y+ky
			if px < 0 || py < 0 || px >= img.Bounds().Dx() || py >= img.Bounds().Dy() { continue }

			off := py*img.Stride + px*4
			pr, pg, pb, pa := img.Pix[off], img.Pix[off+1], img.Pix[off+2], img.Pix[off+3]
			lum := 0.2126*float64(pr) + 0.7152*float64(pg) + 0.0722*float64(pb)

			// Gradient weights
			gx += lum * float64(kx)
			gy += lum * float64(ky)

			rS += uint64(pr); gS += uint64(pg); bS += uint64(pb); aS += uint64(pa); lS += uint64(lum)
			count++
		}
	}
	if count > 0 {
		div := float64(count)
		r, g, b, a = uint8(rS/count), uint8(gS/count), uint8(bS/count), uint8(aS/count)
		lumNorm = (float64(lS) / div) / 255.0
	}
	return
}
