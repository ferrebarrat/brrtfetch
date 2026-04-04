package main

import (
    "bytes"
    "image"
)

// Renderer defines how a frame is converted to terminal strings.
type Renderer interface {
    Render(buf *bytes.Buffer, img *image.RGBA, cfg Config)
}

// IsImageProtocol returns true for renderers that use terminal image protocols
// (kitty, iterm, wezterm). These output binary blobs that must not be
// line-split or re-transmitted every frame.
func IsImageProtocol(mode string) bool {
    switch mode {
    case "kitty", "iterm", "wezterm":
        return true
    default:
        return false
    }
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
