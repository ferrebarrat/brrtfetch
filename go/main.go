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

	wPtr := flag.Int("width", 0, "Fixed width in columns. Overrides -scale for the horizontal axis. 0 = use -scale")
	hPtr := flag.Int("height", -1, "Fixed height in rows. -1 = auto (derive from width and GIF aspect ratio). Overrides -scale for the vertical axis")
	sPtr := flag.Int("scale", 40, "Scale the GIF to this % of the terminal size. Ignored per axis when -width/-height is set")
	fPtr := flag.Int("fps", 20, "Playback speed in frames per second")
	cPtr := flag.Bool("color", true, "Enable color output. Set to false for monochrome")
	mPtr := flag.Float64("multiplier", 1.2, "Brightness multiplier applied to pixel values")
	diPtr := flag.Float64("dither", 0, "Dither intensity. 0 = off, higher values add more dithering (try 0.5-1.0)")
	iPtr := flag.String("info", "", "System info source shown next to the GIF.\n\"\" (empty) = builtin fetcher with interactive slides.\nOr a shell command, e.g. 'fastfetch --logo-type none'")
	oPtr := flag.Int("offset", 0, "Vertical offset in rows before the info text starts (shifts info down)")
	rPtr := flag.String("render", "half-block", "Render mode. Text modes: half-block, block, quadrant, braille, dots, sextant, medusa.\nEffect modes: matrix, crt. Image protocols: kitty, iterm, wezterm.\nExternal: chafa (requires chafa installed)")
	benchPtr := flag.Bool("benchmark", false, "Render one frame, print the time taken, and exit")
	shellPtr := flag.Bool("shell", false, "Run an interactive shell below the animation bar")
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

	rawGif := loadRawGif(gifPath)
	termW, termH := getTerminalSize()
	currentCfg := resolveDimensions(baseCfg, *wPtr, *hPtr, *sPtr, termW, termH, rawGif.Config.Width, rawGif.Config.Height)

	prerendered := getFrameSequence(rawGif, gifPath, currentCfg)

	// Info source: builtin fetcher slideshow or external command
	var slideShow *SlideShow
	var sysInfo []string

	if *iPtr == "" {
		// Builtin fetcher
		slideShow = NewSlideShow([]Slide{
			NewSystemSlide(),
			NewCPUSlide(),
			NewRAMSlide(),
			NewDiskSlide(),
		})
	} else {
		sysInfo = getCommandOutputLines(*iPtr)
	}

	if *shellPtr && !*benchPtr {
		runShellMode(baseCfg, currentCfg, prerendered, sysInfo, slideShow, rawGif, gifPath,
			*wPtr, *hPtr, *sPtr, *oPtr, termW, termH)
		return
	}

	runLegacyMode(baseCfg, currentCfg, prerendered, sysInfo, slideShow, rawGif, gifPath,
		*wPtr, *hPtr, *sPtr, *oPtr, *benchPtr, startTime, termW, termH)
}

func runLegacyMode(baseCfg, currentCfg Config, prerendered [][]byte, sysInfo []string, slideShow *SlideShow,
	rawGif *gif.GIF, gifPath string, flagW, flagH, flagS, offset int,
	bench bool, startTime time.Time, termW, termH int) {

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGWINCH)

	if !bench {
		fmt.Print("\033[?1049h" + ANSI_HIDE_CURSOR + ANSI_DISABLE_WRAP)
	}

	// Enable mouse for slideshow arrow clicks
	if slideShow != nil && !bench {
		slideShow.InfoXOffset = currentCfg.Width + 3
		fmt.Print(ANSI_MOUSE_ON)
		stop := make(chan struct{})
		defer close(stop)
		slideShow.StartRefreshLoop(time.Second, stop)
		go legacyMouseReader(slideShow)
	}

	writer := bufio.NewWriterSize(os.Stdout, 128*1024)
	ticker := time.NewTicker(time.Second / time.Duration(baseCfg.FPS))

	var prevFrameLines [][]byte
	frameIdx := 0
	lastSafeIdxLeg := -1
	isImgProtoLeg := IsImageProtocol(baseCfg.RenderMode)
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
				newCfg := resolveDimensions(baseCfg, flagW, flagH, flagS, termW, termH, rawGif.Config.Width, rawGif.Config.Height)
				writer.WriteString("\033[2J\033[H" + ANSI_DISABLE_WRAP)
				prevFrameLines = nil
				if newCfg.Width != currentCfg.Width || newCfg.Height != currentCfg.Height {
					currentCfg = newCfg
					prerendered = getFrameSequence(rawGif, gifPath, currentCfg)
					frameIdx = 0
					if slideShow != nil {
						slideShow.InfoXOffset = currentCfg.Width + 3
					}
				}
			} else {
				if !bench {
					if slideShow != nil {
						fmt.Print(ANSI_MOUSE_OFF)
					}
					fmt.Print("\033[?1049l" + ANSI_SHOW_CURSOR + ANSI_ENABLE_WRAP)
				}
				os.Exit(0)
			}
		case <-ticker.C:
			if len(prerendered) == 0 {
				continue
			}
			safeIdx := frameIdx % len(prerendered)

			// Get info lines: from slideshow or static
			infoLines := sysInfo
			var legGifH int
			if isImgProtoLeg {
				legGifH = currentCfg.Height
			} else {
				legGifH = len(bytes.Split(prerendered[safeIdx], []byte("\n")))
			}
			if slideShow != nil {
				infoWidth := termW - currentCfg.Width - 3
				if infoWidth < 10 {
					infoWidth = 10
				}
				infoLines = slideShow.RenderCurrent(infoWidth, legGifH)
			}

			if isImgProtoLeg {
				// Image protocol: write image blob only on frame change
				if safeIdx != lastSafeIdxLeg {
					writer.WriteString(ANSI_HOME)
					writer.Write(prerendered[safeIdx])
					lastSafeIdxLeg = safeIdx
				}
				// Always update sysinfo lines
				currentFrameLines := composeInfoOnly(infoLines, offset, currentCfg.Width, legGifH, termW, termH)
				for y, line := range currentFrameLines {
					writer.WriteString(MoveCursor(1+y, 1))
					writer.Write(line)
					writer.WriteString(ANSI_CLEAR_LINE)
				}
				prevFrameLines = currentFrameLines
			} else {
				currentFrameLines := composeFrame(prerendered[safeIdx], infoLines, offset, currentCfg.Width, termW, termH)

				writer.WriteString(ANSI_HOME)
				maxH := len(currentFrameLines)
				if len(prevFrameLines) > maxH {
					maxH = len(prevFrameLines)
				}
				for y := 0; y < maxH; y++ {
					var currLine, prevLine []byte
					if y < len(currentFrameLines) {
						currLine = currentFrameLines[y]
					}
					if y < len(prevFrameLines) {
						prevLine = prevFrameLines[y]
					}
					if bytes.Equal(currLine, prevLine) {
						writer.WriteString(ANSI_CURSOR_DOWN)
					} else if len(currLine) > 0 {
						writer.Write(currLine)
						writer.WriteString(ANSI_CLEAR_LINE + "\r\n")
					} else {
						writer.WriteString(ANSI_CLEAR_LINE + "\r\n")
					}
				}
				prevFrameLines = currentFrameLines
			}

			writer.Flush()

			if bench {
				duration := time.Since(startTime)
				fmt.Printf("\n\x1b[1mBenchmark:\x1b[0m First frame fully rendered in %v\n", duration)
				return
			}

			frameIdx++
		}
	}
}

// legacyMouseReader reads stdin for mouse clicks in legacy (non-shell) mode.
// gifWidth is used to offset click x-coordinates to the info area.
func legacyMouseReader(slideShow *SlideShow) {
	buf := make([]byte, 256)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return
		}
		data := buf[:n]
		for i := 0; i < len(data); {
			if i+2 < len(data) && data[i] == 0x1b && data[i+1] == '[' && data[i+2] == '<' {
				end := i + 3
				for end < len(data) && data[end] != 'M' && data[end] != 'm' {
					end++
				}
				if end < len(data) && data[end] == 'M' {
					// Press event - parse btn;x;y
					parseLegacyClick(string(data[i+3:end]), slideShow)
				}
				i = end + 1
			} else if data[i] == 'q' || data[i] == 0x1b {
				// q or bare ESC to quit
				if data[i] == 'q' {
					if slideShow != nil {
						fmt.Print(ANSI_MOUSE_OFF)
					}
					fmt.Print("\033[?1049l" + ANSI_SHOW_CURSOR + ANSI_ENABLE_WRAP)
					os.Exit(0)
				}
				i++
			} else {
				i++
			}
		}
	}
}

func parseLegacyClick(seq string, slideShow *SlideShow) {
	var btn, x, y int
	fmt.Sscanf(seq, "%d;%d;%d", &btn, &x, &y)
	if btn != 0 {
		return // only handle left click (btn=0)
	}
	termW, _ := getTerminalSize()
	xOff := slideShow.InfoXOffset
	infoW := termW - xOff
	if infoW < 10 {
		infoW = 10
	}
	ls, le, rs, re := slideShow.NavClickRegion(infoW)
	infoX := x - xOff
	if y <= 2 { // nav bar is in first rows of info area
		if infoX >= ls && infoX <= le {
			slideShow.Prev()
		} else if infoX >= rs && infoX <= re {
			slideShow.Next()
		}
	}
}

func runShellMode(baseCfg, currentCfg Config, prerendered [][]byte, sysInfo []string, slideShow *SlideShow,
	rawGif *gif.GIF, gifPath string, flagW, flagH, flagS, offset, termW, termH int) {

	isImgProto := IsImageProtocol(baseCfg.RenderMode)

	// Get initial info lines
	infoLines := sysInfo
	var gifH int
	if isImgProto {
		gifH = currentCfg.Height
	} else {
		gifH = len(bytes.Split(prerendered[0], []byte("\n")))
	}
	if slideShow != nil {
		infoWidth := termW - currentCfg.Width - 3
		if infoWidth < 10 {
			infoWidth = 10
		}
		infoLines = slideShow.RenderCurrent(infoWidth, gifH)
	}

	// Determine bar height
	var barHeight int
	if isImgProto {
		barHeight = gifH
		if len(infoLines)+offset > barHeight {
			barHeight = len(infoLines) + offset
		}
		if barHeight > termH/2 {
			barHeight = termH / 2
		}
	} else {
		testFrame := composeFrame(prerendered[0], infoLines, offset, currentCfg.Width, termW, termH)
		barHeight = len(testFrame)
		if barHeight > termH/2 {
			barHeight = termH / 2
		}
	}
	if barHeight < 1 {
		barHeight = 1
	}

	fmt.Print(ANSI_HIDE_CURSOR + ANSI_DISABLE_WRAP)
	// Clear screen and position for initial render
	fmt.Print("\033[2J" + ANSI_HOME)
	// Set scroll region so shell output doesn't scroll into bar area
	fmt.Print(SetScrollRegion(barHeight+1, termH))

	session, err := NewShellSession(barHeight, termW, termH)
	if err != nil {
		fmt.Print(ANSI_SHOW_CURSOR + ANSI_ENABLE_WRAP)
		fmt.Fprintf(os.Stderr, "brrtfetch: shell mode failed: %v, falling back to legacy mode\n", err)
		startTime := time.Now()
		runLegacyMode(baseCfg, currentCfg, prerendered, sysInfo, slideShow, rawGif, gifPath,
			flagW, flagH, flagS, offset, false, startTime, termW, termH)
		return
	}

	// Pass slideshow to shell session for click handling
	if slideShow != nil {
		session.slideShow = slideShow
		session.infoXOffset = currentCfg.Width + 3
		stop := make(chan struct{})
		defer close(stop)
		slideShow.StartRefreshLoop(time.Second, stop)
	}

	session.Run()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGWINCH)

	writer := bufio.NewWriterSize(os.Stdout, 128*1024)
	ticker := time.NewTicker(time.Second / time.Duration(baseCfg.FPS))
	defer ticker.Stop()

	var prevBarLines [][]byte
	frameIdx := 0
	lastSafeIdx := -1
	var resizeTimer *time.Timer
	lastRender := time.Now()
	minInterval := time.Millisecond * 8

	for {
		select {
		case <-session.done:
			session.Cleanup()
			return

		case sig := <-sigs:
			if sig == syscall.SIGWINCH {
				if resizeTimer != nil {
					resizeTimer.Stop()
				}
				resizeTimer = time.AfterFunc(200*time.Millisecond, func() { sigs <- syscall.SIGUSR1 })
			} else if sig == syscall.SIGUSR1 {
				termW, termH = getTerminalSize()
				newCfg := resolveDimensions(baseCfg, flagW, flagH, flagS, termW, termH, rawGif.Config.Width, rawGif.Config.Height)

				if newCfg.Width != currentCfg.Width || newCfg.Height != currentCfg.Height {
					currentCfg = newCfg
					prerendered = getFrameSequence(rawGif, gifPath, currentCfg)
					frameIdx = 0
				}

				il := sysInfo
				var resizeGifH int
				if isImgProto {
					resizeGifH = currentCfg.Height
				} else {
					resizeGifH = len(bytes.Split(prerendered[frameIdx%len(prerendered)], []byte("\n")))
				}
				if slideShow != nil {
					iw := termW - currentCfg.Width - 3
					if iw < 10 {
						iw = 10
					}
					il = slideShow.RenderCurrent(iw, resizeGifH)
				}
				if isImgProto {
					barHeight = resizeGifH
					if len(il)+offset > barHeight {
						barHeight = len(il) + offset
					}
					if barHeight > termH/2 {
						barHeight = termH / 2
					}
				} else {
					newTest := composeFrame(prerendered[frameIdx%len(prerendered)], il, offset, currentCfg.Width, termW, termH)
					barHeight = len(newTest)
					if barHeight > termH/2 {
						barHeight = termH / 2
					}
				}
				if barHeight < 1 {
					barHeight = 1
				}
				prevBarLines = nil
				lastSafeIdx = -1

				// Clear entire screen on resize and re-apply scroll region
				writer.WriteString("\033[2J")
				writer.WriteString(SetScrollRegion(barHeight+1, termH))
				writer.Flush()

				if slideShow != nil {
					session.infoXOffset = currentCfg.Width + 3
				}
				session.Resize(termW, termH, barHeight)
			} else {
				session.Cleanup()
				os.Exit(0)
			}

		case <-ticker.C:
			if len(prerendered) == 0 {
				continue
			}
			safeIdx := frameIdx % len(prerendered)

			il := sysInfo
			var tickGifH int
			if isImgProto {
				tickGifH = currentCfg.Height
			} else {
				tickGifH = len(bytes.Split(prerendered[safeIdx], []byte("\n")))
			}
			if slideShow != nil {
				iw := termW - currentCfg.Width - 3
				if iw < 10 {
					iw = 10
				}
				il = slideShow.RenderCurrent(iw, tickGifH)
			}

			if isImgProto {
				// Image protocol: write image blob only on frame change
				if safeIdx != lastSafeIdx {
					writer.WriteString(MoveCursor(1, 1))
					writer.Write(prerendered[safeIdx])
					lastSafeIdx = safeIdx
				}
				// Always update sysinfo lines
				currentBarLines := composeInfoOnly(il, offset, currentCfg.Width, tickGifH, termW, barHeight+1)
				for y, line := range currentBarLines {
					writer.WriteString(MoveCursor(1+y, 1))
					writer.Write(line)
					writer.WriteString(ANSI_CLEAR_LINE)
				}
				prevBarLines = currentBarLines
			} else {
				currentBarLines := composeFrame(prerendered[safeIdx], il, offset, currentCfg.Width, termW, barHeight+1)
				for y, line := range currentBarLines {
					if y < len(prevBarLines) && bytes.Equal(line, prevBarLines[y]) {
						continue
					}
					writer.WriteString(MoveCursor(1+y, 1))
					writer.Write(line)
					writer.WriteString(ANSI_CLEAR_LINE)
				}
				prevBarLines = currentBarLines
			}

			// Draw shell region from VTE
			session.RenderShellRegion(writer)
			writer.Flush()
			lastRender = time.Now()
			frameIdx++

		case <-session.dirty:
			if time.Since(lastRender) < minInterval {
				continue
			}
			session.RenderShellRegion(writer)
			writer.Flush()
			lastRender = time.Now()
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
			if cfg.DitherIntensity > 0 {
				applyDithering(proc, cfg.DitherIntensity)
			}
			jobs <- RenderJob{Index: i, Image: proc, PoolKey: proc}
		}
		close(jobs)
	}()

	res := make([][]byte, len(g.Image))
	done := make(chan bool)
	go func() {
		for r := range results {
			res[r.Index] = r.Data
		}
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
	if h > maxH {
		h = maxH
		w = h * gifRatio
	}

	if flagW > 0 {
		cfg.Width = flagW
		cfg.Height = int(float64(cfg.Width) / gifRatio)
	} else if flagH > 0 {
		cfg.Height = flagH
		cfg.Width = int(float64(cfg.Height) * gifRatio)
	} else {
		cfg.Width, cfg.Height = int(w), int(h)
	}
	if cfg.Width < 2 {
		cfg.Width = 2
	}
	if cfg.Height < 1 {
		cfg.Height = 1
	}
	return cfg
}

// composeInfoOnly creates lines with blank space for the image area + sysinfo text.
// Used by image-protocol renderers (kitty/iterm/wezterm) where image data is written separately.
func composeInfoOnly(sysInfo []string, offset int, gifWidth, gifHeight int, termW, termH int) [][]byte {
	totalH := gifHeight
	if len(sysInfo)+offset > totalH {
		totalH = len(sysInfo) + offset
	}
	if totalH > termH-1 {
		totalH = termH - 1
	}

	allowedTextWidth := termW - gifWidth - 3
	if allowedTextWidth < 0 {
		allowedTextWidth = 0
	}

	result := make([][]byte, totalH)
	for y := 0; y < totalH; y++ {
		buf := lineBufferPool.Get().(*bytes.Buffer)
		buf.Reset()
		// Move cursor past image area without overwriting (preserves kitty/iterm/wezterm images)
		fmt.Fprintf(buf, "\x1b[%dG", gifWidth+1)
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

func composeFrame(frameData []byte, sysInfo []string, offset int, gifWidth int, termW, termH int) [][]byte {
	gifLines := bytes.Split(frameData, []byte("\n"))
	totalH := len(gifLines)
	if len(sysInfo)+offset > totalH {
		totalH = len(sysInfo) + offset
	}
	if totalH > termH-1 {
		totalH = termH - 1
	}

	allowedTextWidth := termW - gifWidth - 3
	if allowedTextWidth < 0 {
		allowedTextWidth = 0
	}

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
	if cached, err := loadCache(cachePath, cfg); err == nil {
		return cached
	}
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
		if v < 0 {
			return 0
		}
		if v > 255 {
			return 255
		}
		return uint8(v)
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			idx := y*img.Stride + x*4
			if img.Pix[idx+3] < 128 {
				continue
			}
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
			diffuse(x+1, y, 7.0/16.0)
			diffuse(x-1, y+1, 3.0/16.0)
			diffuse(x, y+1, 5.0/16.0)
			diffuse(x+1, y+1, 1.0/16.0)
		}
	}
}
