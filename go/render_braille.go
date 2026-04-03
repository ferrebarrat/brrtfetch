package main

import (
	"bytes"
	"fmt"
	"image"
)

type BrailleRenderer struct{}

func (br *BrailleRenderer) Render(buf *bytes.Buffer, img *image.RGBA, cfg Config) {
	bounds := img.Bounds()
	imgW, imgH := bounds.Dx(), bounds.Dy()

	// 2x4 dots per cell
	dotW, dotH := cfg.Width*2, cfg.Height*4
	scaleX := float64(imgW) / float64(dotW)
	scaleY := float64(imgH) / float64(dotH)

	// Create a grayscale "energy" map to perform dithering
	// This prevents the "transparent/ghosting" look
	energyMap := make([]float64, dotW*dotH)
	for dy := 0; dy < dotH; dy++ {
		for dx := 0; dx < dotW; dx++ {
			ix, iy := int(float64(dx)*scaleX), int(float64(dy)*scaleY)
			if ix < imgW && iy < imgH {
				off := iy*img.Stride + ix*4
				// Only use pixels that aren't fully transparent
				if img.Pix[off+3] > 10 {
					lum := 0.2126*float64(img.Pix[off]) + 0.7152*float64(img.Pix[off+1]) + 0.0722*float64(img.Pix[off+2])
					energyMap[dy*dotW+dx] = lum
				}
			}
		}
	}

	// Sub-pixel bit order for Unicode Braille
	dotBits := [8][2]int{
		{0, 0}, {0, 1}, {0, 2}, {1, 0},
		{1, 1}, {1, 2}, {0, 3}, {1, 3},
	}

	for y := 0; y < cfg.Height; y++ {
		buf.WriteString("\x1b[0m")

		for x := 0; x < cfg.Width; x++ {
			var brailleChar rune = 0x2800
			var rSum, gSum, bSum, aSum uint64
			var count uint64

			for i, pos := range dotBits {
				dx, dy := x*2+pos[0], y*4+pos[1]
				if dx >= dotW || dy >= dotH { continue }

				idx := dy*dotW + dx
				val := energyMap[idx]

				// Boosted thresholding: if it has any significant energy, show it.
				// We also use a lower threshold (60) than standard (128) to make it "solid"
				threshold := 1.0 / cfg.Multiplier
				if val > threshold {
					brailleChar |= (1 << uint(i))
				}

				// Sample original colors for the cell foreground
				origX, origY := int(float64(dx)*scaleX), int(float64(dy)*scaleY)
				off := origY*img.Stride + origX*4
				rSum += uint64(img.Pix[off])
				gSum += uint64(img.Pix[off+1])
				bSum += uint64(img.Pix[off+2])
				aSum += uint64(img.Pix[off+3])
				count++
			}

			if count > 0 && brailleChar != 0x2800 {
				if cfg.Color {
					fmt.Fprintf(buf, "\x1b[38;2;%d;%d;%dm%c", rSum/count, gSum/count, bSum/count, brailleChar)
				} else {
					buf.WriteRune(brailleChar)
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
