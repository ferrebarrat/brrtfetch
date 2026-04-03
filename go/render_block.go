package main

import (
	"bytes"
	"fmt"
	"image"
	"math"
)

// Renamed to blockGlyph to avoid collision with Medusa
type blockGlyph struct {
	char   string  
	mask   uint16
	weight float64
}

// Renamed to blockGlyphs
var blockGlyphs = []blockGlyph{
	{" ", 0b0000_0000_0000_0000, 0.0},
	{"░", 0b1010_0101_1010_0101, 0.25},
	{"▒", 0b1101_0110_1011_0110, 0.5},
	{"▓", 0b1111_0111_1111_1101, 0.75},
	{"█", 0b1111_1111_1111_1111, 1.0},
	{"▀", 0b1111_1111_0000_0000, 0.5},
	{"▄", 0b0000_0000_1111_1111, 0.5},
	{"▌", 0b1100_1100_1100_1100, 0.5},
	{"▐", 0b0011_0011_0011_0011, 0.5},
}

type BlockRenderer struct{}

func (br *BlockRenderer) Render(buf *bytes.Buffer, img *image.RGBA, cfg Config) {
	bounds := img.Bounds()
	imgW, imgH := bounds.Dx(), bounds.Dy()
	stepX, stepY := float64(imgW)/float64(cfg.Width), float64(imgH)/float64(cfg.Height)

	for y := 0; y < cfg.Height; y++ {
		buf.WriteString("\x1b[0m\x1b[1m")

		for x := 0; x < cfg.Width; x++ {
			xS, yS := int(float64(x)*stepX), int(float64(y)*stepY)
			xE, yE := int(float64(x+1)*stepX), int(float64(y+1)*stepY)

			targetMask, red, green, blue, alpha, lum := br.analyzeCell(img, xS, yS, xE, yE)

			if alpha < 100 || (lum*cfg.Multiplier) < 0.05 {
				buf.WriteByte(' ')
				continue
			}

			boostedLum := math.Pow(lum, 0.5) * cfg.Multiplier
			if boostedLum > 1.0 { boostedLum = 1.0 }

			bestChar := br.findBestBlockChar(targetMask, boostedLum)

			if cfg.Color {
				fmt.Fprintf(buf, "\x1b[38;2;%d;%d;%dm%s", red, green, blue, bestChar)
			} else {
				buf.WriteString(bestChar)
			}
		}
		buf.WriteString("\x1b[0m")
		if y < cfg.Height-1 {
			buf.WriteByte('\n')
		}
	}
}

func (br *BlockRenderer) analyzeCell(img *image.RGBA, x1, y1, x2, y2 int) (uint16, uint8, uint8, uint8, uint8, float64) {
	var mask uint16
	var rSum, gSum, bSum, aSum, tLum uint64
	count := 0

	for py := y1; py < y2; py++ {
		for px := x1; px < x2; px++ {
			if px >= img.Bounds().Dx() || py >= img.Bounds().Dy() { continue }
			off := py*img.Stride + px*4
			l := 0.2126*float64(img.Pix[off]) + 0.7152*float64(img.Pix[off+1]) + 0.0722*float64(img.Pix[off+2])
			
			rSum += uint64(img.Pix[off])
			gSum += uint64(img.Pix[off+1])
			bSum += uint64(img.Pix[off+2])
			aSum += uint64(img.Pix[off+3])
			tLum += uint64(l)
			count++
		}
	}
	if count == 0 { return 0, 0, 0, 0, 0, 0 }

	avgLumNorm := (float64(tLum) / float64(count)) / 255.0

	for sy := 0; sy < 4; sy++ {
		for sx := 0; sx < 4; sx++ {
			subX := x1 + (sx * (x2 - x1) / 4)
			subY := y1 + (sy * (y2 - y1) / 4)
			if subX >= img.Bounds().Dx() || subY >= img.Bounds().Dy() { continue }
			off := subY*img.Stride + subX*4
			pxL := (0.2126*float64(img.Pix[off]) + 0.7152*float64(img.Pix[off+1]) + 0.0722*float64(img.Pix[off+2])) / 255.0
			if pxL >= avgLumNorm*0.9 {
				mask |= (1 << uint(sy*4+sx))
			}
		}
	}
	return mask, uint8(rSum/uint64(count)), uint8(gSum/uint64(count)), uint8(bSum/uint64(count)), uint8(aSum/uint64(count)), avgLumNorm
}

func (br *BlockRenderer) findBestBlockChar(mask uint16, targetWeight float64) string {
	bestIdx := 0
	minScore := math.MaxFloat64

	for i, g := range blockGlyphs {
		shapeDiff := float64(blockPopcount(mask ^ g.mask)) / 16.0
		weightDiff := math.Abs(targetWeight - g.weight)
		score := (shapeDiff * 0.2) + (weightDiff * 0.8)
		if score < minScore {
			minScore = score
			bestIdx = i
		}
	}
	return blockGlyphs[bestIdx].char
}

func blockPopcount(x uint16) int {
	x -= (x >> 1) & 0x5555
	x = (x & 0x3333) + ((x >> 2) & 0x3333)
	return int(((x + (x >> 4)) & 0x0F0F) * 0x0101) >> 8
}
