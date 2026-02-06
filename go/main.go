package main

import (
    "bufio"
    "bytes"
    "flag"
    "fmt"
    "image"
    "image/color"
    "image/draw"
    "image/gif"
    "os"
    "os/signal"
    "path/filepath"
    "runtime"
    "sync"
    "syscall"
    "time"
)

var (
    imageBufferPool chan *image.RGBA
    lineBufferPool  = sync.Pool{New: func() interface{} { return new(bytes.Buffer) }}
)

func main() {
	// Start the timer immediately to capture flag parsing and GIF loading
	startTime := time.Now()

	wPtr := flag.Int("width", 0, "Width")
	hPtr := flag.Int("height", -1, "Height")
	sPtr := flag.Int("scale", 40, "Scale %")
	fPtr := flag.Int("fps", 20, "FPS")
	cPtr := flag.Bool("color", true, "Color")
	mPtr := flag.Float64("multiplier", 1.2, "Brightness")
	diPtr := flag.Float64("dither", 0, "Dither")
	iPtr := flag.String("info", "fastfetch --logo-type none", "Info cmd")
	oPtr := flag.Int("offset", 0, "Top offset")
	rPtr := flag.String("render", "half-block", "Render type: half-block")
	benchPtr := flag.Bool("benchmark", false, "Print time to first frame and exit")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Println("Usage: brrtfetch [options] file.gif")
		fmt.Println("Use 'brrtfetch --help' for more info.")
		return
	}

	gifPath, _ := filepath.Abs(flag.Arg(0))
	baseCfg := Config{
		FPS: *fPtr, Color: *cPtr, DitherIntensity: *diPtr,
		Multiplier: *mPtr, RenderMode: *rPtr,
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGWINCH)

	rawGif := loadRawGif(gifPath)
	termW, termH := getTerminalSize()
	currentCfg := resolveDimensions(baseCfg, *wPtr, *hPtr, *sPtr, termW, termH, rawGif.Config.Width, rawGif.Config.Height)

	prerendered := getFrameSequence(rawGif, gifPath, currentCfg)
	sysInfo := getCommandOutputLines(*iPtr)

	// Skip alternate buffer in benchmark mode so we can see the result in history
	if !*benchPtr {
		fmt.Print("\033[?1049h" + ANSI_HIDE_CURSOR + ANSI_DISABLE_WRAP)
	}

	writer := bufio.NewWriterSize(os.Stdout, 128*1024)
	ticker := time.NewTicker(time.Second / time.Duration(baseCfg.FPS))

	var prevFrameLines [][]byte
	frameIdx := 0
	var resizeTimer *time.Timer

	for {
		select {
		case sig := <-sigs:
			if sig == syscall.SIGWINCH {
				if resizeTimer != nil {
					resizeTimer.Stop()
				}
				resizeTimer = time.AfterFunc(200*time.Millisecond, func() { sigs <- syscall.SIGUSR1 })
			} else if sig == syscall.SIGUSR1 {
				termW, termH = getTerminalSize()
				newCfg := resolveDimensions(baseCfg, *wPtr, *hPtr, *sPtr, termW, termH, rawGif.Config.Width, rawGif.Config.Height)
				writer.WriteString("\033[2J\033[H" + ANSI_DISABLE_WRAP)
				if newCfg.Width != currentCfg.Width || newCfg.Height != currentCfg.Height {
					currentCfg = newCfg
					prerendered = getFrameSequence(rawGif, gifPath, currentCfg)
					frameIdx = 0
					prevFrameLines = nil
				}
			} else {
				if !*benchPtr {
					fmt.Print("\033[?1049l" + ANSI_SHOW_CURSOR + ANSI_ENABLE_WRAP)
				}
				os.Exit(0)
			}
		case <-ticker.C:
			if len(prerendered) == 0 {
				continue
			}
			safeIdx := frameIdx % len(prerendered)
			currentFrameLines := composeFrame(prerendered[safeIdx], sysInfo, *oPtr, currentCfg.Width, termW, termH)

			writer.WriteString(ANSI_HOME)
			for y, line := range currentFrameLines {
				if y < len(prevFrameLines) && bytes.Equal(line, prevFrameLines[y]) {
					writer.WriteString(ANSI_CURSOR_DOWN)
				} else {
					writer.Write(line)
					writer.WriteString(ANSI_CLEAR_LINE + "\r\n")
				}
			}
			
			// The timer stops only after the buffer is flushed to the terminal
			writer.Flush()

			if *benchPtr {
				duration := time.Since(startTime)
				fmt.Printf("\n\x1b[1mBenchmark:\x1b[0m First frame fully rendered in %v\n", duration)
				return
			}

			prevFrameLines = currentFrameLines
			frameIdx++
		}
	}
}
// Logic implementations moved back into main for orchestration
func processGif(g *gif.GIF, cfg Config) [][]byte {
    numWorkers := runtime.NumCPU()
    jobs := make(chan RenderJob, len(g.Image))
    results := make(chan RenderResult, len(g.Image))
    var wg sync.WaitGroup
    
    renderer := GetRenderer(cfg.RenderMode)

    imageBufferPool = make(chan *image.RGBA, numWorkers*2)
    for i := 0; i < cap(imageBufferPool); i++ {
        imageBufferPool <- image.NewRGBA(image.Rect(0, 0, g.Config.Width, g.Config.Height))
    }

    for i := 0; i < numWorkers; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for job := range jobs {
                buf := lineBufferPool.Get().(*bytes.Buffer)
                buf.Reset()
                renderer.Render(buf, job.Image, cfg)
                resBytes := make([]byte, buf.Len())
                copy(resBytes, buf.Bytes())
                results <- RenderResult{Index: job.Index, Data: resBytes}
                lineBufferPool.Put(buf)
                imageBufferPool <- job.PoolKey
            }
        }()
    }

    go func() {
        fullFrame := image.NewRGBA(image.Rect(0, 0, g.Config.Width, g.Config.Height))
        snapshot := image.NewRGBA(fullFrame.Bounds())
        var lastDisposal int
        var lastBounds image.Rectangle

        for i, frame := range g.Image {
            if lastDisposal == gif.DisposalPrevious {
                draw.Draw(fullFrame, fullFrame.Bounds(), snapshot, image.Point{}, draw.Src)
            } else if lastDisposal != gif.DisposalNone {
                draw.Draw(fullFrame, lastBounds, image.NewUniform(color.Transparent), image.Point{}, draw.Src)
            }
            if int(g.Disposal[i]) == gif.DisposalPrevious {
                copy(snapshot.Pix, fullFrame.Pix)
            }
            draw.Draw(fullFrame, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)
            lastDisposal = int(g.Disposal[i])
            lastBounds = frame.Bounds()

            proc := <-imageBufferPool
            copy(proc.Pix, fullFrame.Pix)
            if cfg.DitherIntensity > 0 { applyDithering(proc, cfg.DitherIntensity) }
            jobs <- RenderJob{Index: i, Image: proc, PoolKey: proc}
        }
        close(jobs)
    }()

    res := make([][]byte, len(g.Image))
    done := make(chan bool)
    go func() {
        for r := range results { res[r.Index] = r.Data }
        done <- true
    }()
    wg.Wait()
    close(results)
    <-done
    return res
}

func resolveDimensions(base Config, flagW, flagH, flagS, termW, termH, gifW, gifH int) Config {
    cfg := base
    if flagW > 0 && flagH > 0 {
        cfg.Width, cfg.Height = flagW, flagH
        return cfg
    }
    maxW, maxH := float64(termW)*(float64(flagS)/100.0), float64(termH)*(float64(flagS)/100.0)
    gifRatio := float64(gifW) / (float64(gifH) / 2.0)
    w, h := maxW, maxW/gifRatio
    if h > maxH { h = maxH; w = h * gifRatio }

    if flagW > 0 {
        cfg.Width = flagW
        cfg.Height = int(float64(cfg.Width) / gifRatio)
    } else if flagH > 0 {
        cfg.Height = flagH
        cfg.Width = int(float64(cfg.Height) * gifRatio)
    } else {
        cfg.Width, cfg.Height = int(w), int(h)
    }
    if cfg.Width < 2 { cfg.Width = 2 }
    if cfg.Height < 1 { cfg.Height = 1 }
    return cfg
}

func composeFrame(frameData []byte, sysInfo []string, offset int, gifWidth int, termW, termH int) [][]byte {
    gifLines := bytes.Split(frameData, []byte("\n"))
    totalH := len(gifLines)
    if len(sysInfo)+offset > totalH { totalH = len(sysInfo) + offset }
    if totalH > termH-1 { totalH = termH - 1 }

    allowedTextWidth := termW - gifWidth - 3
    if allowedTextWidth < 0 { allowedTextWidth = 0 }

    result := make([][]byte, totalH)
    for y := 0; y < totalH; y++ {
        buf := lineBufferPool.Get().(*bytes.Buffer)
        buf.Reset()
        if y < len(gifLines) && len(gifLines[y]) > 0 {
            buf.Write(gifLines[y])
        } else {
            buf.Write(bytes.Repeat([]byte(" "), gifWidth))
        }
        buf.WriteString("\x1b[0m")
        sIdx := y - offset
        if sIdx >= 0 && sIdx < len(sysInfo) {
            buf.WriteString("   ")
            buf.Write(truncateAnsi([]byte(sysInfo[sIdx]), allowedTextWidth))
        }
        lineCopy := make([]byte, buf.Len())
        copy(lineCopy, buf.Bytes())
        result[y] = lineCopy
        lineBufferPool.Put(buf)
    }
    return result
}

func getFrameSequence(g *gif.GIF, path string, cfg Config) [][]byte {
    cachePath := getCachePath(path, cfg)
    if cached, err := loadCache(cachePath, cfg); err == nil { return cached }
    rendered := processGif(g, cfg)
    saveCache(cachePath, rendered, cfg)
    return rendered
}

func loadRawGif(path string) *gif.GIF {
    f, _ := os.Open(path)
    defer f.Close()
    g, _ := gif.DecodeAll(f)
    return g
}

func applyDithering(img *image.RGBA, intensity float64) {
    bounds := img.Bounds()
    w, h := bounds.Dx(), bounds.Dy()
    clamp := func(v float64) uint8 {
        if v < 0 { return 0 }
        if v > 255 { return 255 }
        return uint8(v)
    }
    for y := 0; y < h; y++ {
        for x := 0; x < w; x++ {
            idx := y*img.Stride + x*4
            if img.Pix[idx+3] < 128 { continue }
            oldR, oldG, oldB := float64(img.Pix[idx]), float64(img.Pix[idx+1]), float64(img.Pix[idx+2])
            qStep := 48.0 * intensity
            newR, newG, newB := float64(int(oldR/qStep+0.5))*qStep, float64(int(oldG/qStep+0.5))*qStep, float64(int(oldB/qStep+0.5))*qStep
            img.Pix[idx], img.Pix[idx+1], img.Pix[idx+2] = clamp(newR), clamp(newG), clamp(newB)
            errR, errG, errB := (oldR-newR)*intensity, (oldG-newG)*intensity, (oldB-newB)*intensity
            diffuse := func(nx, ny int, factor float64) {
                if nx >= 0 && nx < w && ny >= 0 && ny < h {
                    nIdx := ny*img.Stride + nx*4
                    if img.Pix[nIdx+3] > 128 {
                        img.Pix[nIdx] = clamp(float64(img.Pix[nIdx]) + errR*factor)
                        img.Pix[nIdx+1] = clamp(float64(img.Pix[nIdx+1]) + errG*factor)
                        img.Pix[nIdx+2] = clamp(float64(img.Pix[nIdx+2]) + errB*factor)
                    }
                }
            }
            diffuse(x+1, y, 7.0/16.0); diffuse(x-1, y+1, 3.0/16.0); diffuse(x, y+1, 5.0/16.0); diffuse(x+1, y+1, 1.0/16.0)
        }
    }
}
