package main

import (
	"bytes"
	"fmt"
	"image"
	"math"
)

// Unicode Quadrant Characters
// These rely on bitmask logic: (TL << 3) | (TR << 2) | (BL << 1) | (BR << 0)
var quadrantMap = [16]string{
	" ", // 0000: Empty
	"▗", // 0001: BR
	"▖", // 0010: BL
	"▄", // 0011: BL + BR (Lower Half)
	"▝", // 0100: TR
	"▐", // 0101: TR + BR (Right Half)
	"▞", // 0110: TR + BL (Diagonal 2)
	"▟", // 0111: !TL
	"▘", // 1000: TL
	"▚", // 1001: TL + BR (Diagonal 1)
	"▌", // 1010: TL + BL (Left Half)
	"▙", // 1011: !TR
	"▀", // 1100: TL + TR (Upper Half)
	"▜", // 1101: !BL
	"▛", // 1110: !BR
	"█", // 1111: Full
}

type QuadrantRenderer struct{}

func (q *QuadrantRenderer) Render(buf *bytes.Buffer, img *image.RGBA, cfg Config) {
	// We need 2x2 pixels from source for every 1 character
	scaleX := float64(img.Bounds().Dx()) / float64(cfg.Width*2)
	scaleY := float64(img.Bounds().Dy()) / float64(cfg.Height*2)

	for y := 0; y < cfg.Height; y++ {
		for x := 0; x < cfg.Width; x++ {
			// Sample 4 pixels (TL, TR, BL, BR)
			baseX := int(float64(x*2) * scaleX)
			baseY := int(float64(y*2) * scaleY)
			
			// Get RGBA for the 2x2 grid (returns [4]uint8)
			c1 := getPixel(img, baseX, baseY)                       // Top-Left
			c2 := getPixel(img, baseX+int(scaleX), baseY)           // Top-Right
			c3 := getPixel(img, baseX, baseY+int(scaleY))           // Bottom-Left
			c4 := getPixel(img, baseX+int(scaleX), baseY+int(scaleY)) // Bottom-Right

			if !cfg.Color {
				q.renderGrayscale(buf, c1, c2, c3, c4, cfg.Multiplier)
			} else {
				q.renderColor(buf, c1, c2, c3, c4)
			}
		}
		if y < cfg.Height-1 {
			buf.WriteByte('\n')
		}
	}
}

// Fixed: Returns a single [4]uint8 array instead of 4 separate values
func getPixel(img *image.RGBA, x, y int) [4]uint8 {
	if x >= img.Bounds().Dx() || y >= img.Bounds().Dy() {
		return [4]uint8{0, 0, 0, 0}
	}
	idx := y*img.Stride + x*4
	return [4]uint8{img.Pix[idx], img.Pix[idx+1], img.Pix[idx+2], img.Pix[idx+3]}
}

func (q *QuadrantRenderer) renderGrayscale(buf *bytes.Buffer, c1, c2, c3, c4 [4]uint8, mult float64) {
	// Calculate luminance and threshold
	getLum := func(c [4]uint8) bool {
		if c[3] == 0 { return false } // Alpha check
		lum := 0.21*float64(c[0]) + 0.72*float64(c[1]) + 0.07*float64(c[2])
		return lum > (100.0 * mult)
	}

	bitmask := 0
	if getLum(c1) { bitmask |= 8 } // TL
	if getLum(c2) { bitmask |= 4 } // TR
	if getLum(c3) { bitmask |= 2 } // BL
	if getLum(c4) { bitmask |= 1 } // BR

	buf.WriteString(quadrantMap[bitmask])
}

func (q *QuadrantRenderer) renderColor(buf *bytes.Buffer, p1, p2, p3, p4 [4]uint8) {
	// If all pixels are empty/transparent
	if p1[3] == 0 && p2[3] == 0 && p3[3] == 0 && p4[3] == 0 {
		buf.WriteString("\x1b[0m ")
		return
	}

	pixels := [][4]uint8{}
	if p1[3] > 0 { pixels = append(pixels, p1) }
	if p2[3] > 0 { pixels = append(pixels, p2) }
	if p3[3] > 0 { pixels = append(pixels, p3) }
	if p4[3] > 0 { pixels = append(pixels, p4) }

	fg, bg := p1, p1 
	maxDist := -1.0

	if len(pixels) > 0 {
		fg = pixels[0]
		bg = pixels[0]
	}
	
	for i := 0; i < len(pixels); i++ {
		for j := i + 1; j < len(pixels); j++ {
			d := colorDist(pixels[i], pixels[j])
			if d > maxDist {
				maxDist = d
				bg = pixels[i] 
				fg = pixels[j] 
			}
		}
	}

	bitmask := 0
	mapBit := func(p [4]uint8, shift int) {
		if p[3] == 0 { return }
		distToBg := colorDist(p, bg)
		distToFg := colorDist(p, fg)
		if distToFg < distToBg {
			bitmask |= (1 << shift)
		}
	}

	mapBit(p1, 3) 
	mapBit(p2, 2) 
	mapBit(p3, 1) 
	mapBit(p4, 0) 

	char := quadrantMap[bitmask]
	
	fgStr := fmt.Sprintf("\x1b[38;2;%d;%d;%dm", fg[0], fg[1], fg[2])
	bgStr := "\x1b[49m" 
	
	if bg[3] > 0 && bitmask != 15 { 
		bgStr = fmt.Sprintf("\x1b[48;2;%d;%d;%dm", bg[0], bg[1], bg[2])
	}
	
	buf.WriteString(fgStr + bgStr + char)
}

func colorDist(c1, c2 [4]uint8) float64 {
	rd := float64(c1[0]) - float64(c2[0])
	gd := float64(c1[1]) - float64(c2[1])
	bd := float64(c1[2]) - float64(c2[2])
	return math.Sqrt(rd*rd + gd*gd + bd*bd)
}
