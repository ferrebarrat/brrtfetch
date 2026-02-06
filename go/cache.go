package main

import (
    "crypto/md5"
    "encoding/gob"
    "fmt"
    "os"
    "path/filepath"
)

func getCachePath(path string, cfg Config) string {
    info, _ := os.Stat(path)
    hash := md5.Sum([]byte(fmt.Sprintf("%s-%d-%d", path, info.Size(), info.ModTime().UnixNano())))
    return filepath.Join(os.TempDir(), fmt.Sprintf("brrtfetch_%s_%x_%d_%d.bin", cfg.RenderMode, hash, cfg.Width, cfg.Height))
}

func loadCache(path string, cfg Config) ([][]byte, error) {
    f, err := os.Open(path)
    if err != nil { return nil, err }
    defer f.Close()
    var data CacheData
    gob.NewDecoder(f).Decode(&data)
    if data.Config.DitherIntensity != cfg.DitherIntensity || data.Config.RenderMode != cfg.RenderMode {
        return nil, fmt.Errorf("stale")
    }
    return data.Frames, nil
}

func saveCache(path string, frames [][]byte, cfg Config) {
    f, _ := os.Create(path)
    defer f.Close()
    gob.NewEncoder(f).Encode(CacheData{Frames: frames, Config: cfg})
} 
