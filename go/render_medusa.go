package main

import (
	"bytes"
	"fmt"
	"image"
	"math"
)

type charGlyph struct {
	char byte
	mask uint16
	weight float64 // How 'bright' this character feels
}

// Optimized for brightness: using 'heavy' characters that fill the cell
var glyphs = []charGlyph{
	{' ', 0b0000_0000_0000_0000, 0.0},
	{'.', 0b0000_0000_0110_0110, 0.2},
	{'*', 0b0100_1110_0100_0000, 0.4},
	{'=', 0b0000_1111_0000_1111, 0.5},
	{'%', 0b1010_0101_1010_0101, 0.7},
	{'#', 0b1111_1010_1010_1111, 0.8},
	{'W', 0b1010_1010_1010_1111, 0.9},
	{'@', 0b1111_1111_1111_1111, 1.0},
}

type MedusaRenderer struct{}

func (m *MedusaRenderer) Render(buf *bytes.Buffer, img *image.RGBA, cfg Config) {
	bounds := img.Bounds()
	imgW, imgH := bounds.Dx(), bounds.Dy()
	stepX, stepY := float64(imgW)/float64(cfg.Width), float64(imgH)/float64(cfg.Height)

	for y := 0; y < cfg.Height; y++ {
		// Start line: Reset + Bold for extra "pop"
		buf.WriteString("\x1b[0m\x1b[1m")

		for x := 0; x < cfg.Width; x++ {
			xS, yS := int(float64(x)*stepX), int(float64(y)*stepY)
			xE, yE := int(float64(x+1)*stepX), int(float64(y+1)*stepY)

			targetMask, r, g, b, a, lum := m.analyzeCell(img, xS, yS, xE, yE)

			if a < 127 || (lum*cfg.Multiplier) < 0.1 {
				buf.WriteByte(' ')
				continue
			}

			// Contrast boost: Stretch the luminance
			boostedLum := math.Pow(lum, 0.7) * cfg.Multiplier 
			bestChar := m.findBestVividChar(targetMask, boostedLum)

			if cfg.Color {
				// \x1b[1m (Bold) + \x1b[38;2 (TrueColor)
				fmt.Fprintf(buf, "\x1b[38;2;%d;%d;%dm%c", r, g, b, bestChar)
			} else {
				buf.WriteByte(bestChar)
			}
		}
		buf.WriteString("\x1b[0m\n")
	}
}

func (m *MedusaRenderer) analyzeCell(img *image.RGBA, x1, y1, x2, y2 int) (uint16, uint8, uint8, uint8, uint8, float64) {
	var mask uint16
	var r, g, b, a, tLum uint64
	count := 0

	for py := y1; py < y2; py++ {
		for px := x1; px < x2; px++ {
			if px >= img.Bounds().Dx() || py >= img.Bounds().Dy() { continue }
			off := py*img.Stride + px*4
			l := 0.2126*float64(img.Pix[off]) + 0.7152*float64(img.Pix[off+1]) + 0.0722*float64(img.Pix[off+2])
			
			r += uint64(img.Pix[off]); g += uint64(img.Pix[off+1])
			b += uint64(img.Pix[off+2]); a += uint64(img.Pix[off+3])
			tLum += uint64(l)
			count++
		}
	}
	if count == 0 { return 0, 0, 0, 0, 0, 0 }

	avgLumNorm := (float64(tLum) / float64(count)) / 255.0

	// Create bitmask based on local thresholding
	for sy := 0; sy < 4; sy++ {
		for sx := 0; sx < 4; sx++ {
			subX := x1 + (sx * (x2 - x1) / 4)
			subY := y1 + (sy * (y2 - y1) / 4)
			if subX >= img.Bounds().Dx() || subY >= img.Bounds().Dy() { continue }
			off := subY*img.Stride + subX*4
			pxL := (0.2126*float64(img.Pix[off]) + 0.7152*float64(img.Pix[off+1]) + 0.0722*float64(img.Pix[off+2])) / 255.0
			if pxL > avgLumNorm*0.8 { mask |= (1 << uint(sy*4+sx)) }
		}
	}

	return mask, uint8(r/uint64(count)), uint8(g/uint64(count)), uint8(b/uint64(count)), uint8(a/uint64(count)), avgLumNorm
}

func (m *MedusaRenderer) findBestVividChar(mask uint16, targetWeight float64) byte {
	bestIdx := 0
	minScore := math.MaxFloat64

	for i, g := range glyphs {
		// Score = Shape Difference + Weight Difference
		// This forces the renderer to pick 'heavy' characters for bright pixels
		shapeDiff := float64(popcount(mask ^ g.mask)) / 16.0
		weightDiff := math.Abs(targetWeight - g.weight)
		
		score := (shapeDiff * 0.3) + (weightDiff * 0.7)
		if score < minScore {
			minScore = score
			bestIdx = i
		}
	}
	return glyphs[bestIdx].char
}

func popcount(x uint16) int {
	x -= (x >> 1) & 0x5555
	x = (x & 0x3333) + ((x >> 2) & 0x3333)
	return int(((x + (x >> 4)) & 0x0F0F) * 0x0101) >> 8
}
