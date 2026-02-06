package main

import (
    "bytes"
    "image"
)

// Renderer defines how a frame is converted to terminal strings.
type Renderer interface {
    Render(buf *bytes.Buffer, img *image.RGBA, cfg Config)
}

func GetRenderer(mode string) Renderer {
    switch mode {
    case "medusa":
        return &MedusaRenderer{}
    case "sextant":
        return &SextantRenderer{}
    case "matrix":
        return &MatrixRenderer{}
    case "crt":
        return &CRTRenderer{}
    case "dots":
        return &DotRenderer{}
    case "block":
        return &BlockRenderer{}
    case "braille":
        return &BrailleRenderer{}
    case "half-block":
        return &HalfBlockRenderer{}
    case "quadrant":
        return &QuadrantRenderer{}
    case "chafa":
        return &ChafaRenderer{}
    case "wezterm":
        return &WeztermRenderer{}
    case "kitty":
        return &KittyRenderer{}
    case "iterm":
        return &ITermRenderer{}
    default:
        return &HalfBlockRenderer{}
    }
}
