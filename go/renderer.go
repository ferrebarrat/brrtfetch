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
    case "half-block":
        return &HalfBlockRenderer{}
    case "quadrant":
        return &QuadrantRenderer{}
    case "kitty":
        return &KittyRenderer{}
    case "sixel":
        return &SixelRenderer{}
    case "chafa":
        return &ChafaRenderer{}
    default:
        return &HalfBlockRenderer{}
    }
}
