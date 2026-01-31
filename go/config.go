package main

import "image"

type Config struct {
    Width           int
    Height          int
    FPS             int
    Color           bool
    DitherIntensity float64
    Multiplier      float64
    RenderMode      string // e.g., "half-block"
}

type CacheData struct {
    Frames [][]byte
    Config Config
}

type RenderJob struct {
    Index   int
    Image   *image.RGBA
    PoolKey *image.RGBA
}

type RenderResult struct {
    Index int
    Data  []byte
}
