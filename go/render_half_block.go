package main

import (
	"bytes"
	"fmt"
	"image"
)

const (
	UPPER_HALF_BLOCK = "▀"
	LOWER_HALF_BLOCK = "▄"
	FULL_BLOCK       = "█"
)

type HalfBlockRenderer struct{}

func (h *HalfBlockRenderer) Render(buf *bytes.Buffer, img *image.RGBA, cfg Config) {
	scaleX := float64(img.Bounds().Dx()) / float64(cfg.Width)
	scaleY := float64(img.Bounds().Dy()) / float64(cfg.Height*2)

	for y := 0; y < cfg.Height; y++ {
		for x := 0; x < cfg.Width; x++ {
			px := int(float64(x) * scaleX)
			pyT, pyB := int(float64(y*2)*scaleY), int(float64(y*2+1)*scaleY)
			oT, oB := pyT*img.Stride+px*4, pyB*img.Stride+px*4
			
			r1, g1, b1, a1 := img.Pix[oT], img.Pix[oT+1], img.Pix[oT+2], img.Pix[oT+3]
			r2, g2, b2, a2 := img.Pix[oB], img.Pix[oB+1], img.Pix[oB+2], img.Pix[oB+3]

			if !cfg.Color {
				h.renderGrayscale(buf, r1, g1, b1, a1, r2, g2, b2, a2, cfg.Multiplier)
			} else {
				h.renderColor(buf, r1, g1, b1, a1, r2, g2, b2, a2)
			}
		}
		if y < cfg.Height-1 {
			buf.WriteByte('\n')
		}
	}
}

func (h *HalfBlockRenderer) renderGrayscale(buf *bytes.Buffer, r1, g1, b1, a1, r2, g2, b2, a2 uint8, mult float64) {
	lum1 := 0.21*float64(r1) + 0.72*float64(g1) + 0.07*float64(b1)
	lum2 := 0.21*float64(r2) + 0.72*float64(g2) + 0.07*float64(b2)
	thresh := 100.0 * mult
	t, b := a1 > 0 && lum1 > thresh, a2 > 0 && lum2 > thresh
	
	if t && b {
		buf.WriteString(FULL_BLOCK)
	} else if t {
		buf.WriteString(UPPER_HALF_BLOCK)
	} else if b {
		buf.WriteString(LOWER_HALF_BLOCK)
	} else {
		buf.WriteByte(' ')
	}
}

func (h *HalfBlockRenderer) renderColor(buf *bytes.Buffer, r1, g1, b1, a1, r2, g2, b2, a2 uint8) {
	if a1 == 0 && a2 == 0 {
		buf.WriteString("\x1b[0m ")
	} else if a1 > 0 && a2 == 0 {
		fmt.Fprintf(buf, "\x1b[38;2;%d;%d;%dm\x1b[49m%s", r1, g1, b1, UPPER_HALF_BLOCK)
	} else if a1 == 0 && a2 > 0 {
		fmt.Fprintf(buf, "\x1b[38;2;%d;%d;%dm\x1b[49m%s", r2, g2, b2, LOWER_HALF_BLOCK)
	} else {
		fmt.Fprintf(buf, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm%s", r1, g1, b1, r2, g2, b2, UPPER_HALF_BLOCK)
	}
}
