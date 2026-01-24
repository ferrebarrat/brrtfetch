package main

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/gob"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// --- Types ---

type Config struct {
	Width           int
	Height          int
	FPS             int
	Color           bool
	DitherIntensity float64
	Multiplier      float64
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

// --- Globals & Pools ---

var (
	imageBufferPool chan *image.RGBA
	lineBufferPool  = sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	}
)

const (
	ANSI_HIDE_CURSOR  = "\033[?25l"
	ANSI_SHOW_CURSOR  = "\033[?25h"
	ANSI_HOME         = "\033[H"
	ANSI_CURSOR_DOWN  = "\033[1B"
	ANSI_DISABLE_WRAP = "\033[?7l"
	ANSI_ENABLE_WRAP  = "\033[?7h"
	ANSI_CLEAR_LINE   = "\x1b[K"
	UPPER_HALF_BLOCK  = "▀"
	LOWER_HALF_BLOCK  = "▄"
	FULL_BLOCK        = "█"
)

func main() {
	wPtr := flag.Int("width", 0, "Width (0 = auto-scale)")
	hPtr := flag.Int("height", -1, "Height (-1 = auto-aspect)")
	sPtr := flag.Int("scale", 40, "Percentage of screen to use when autoscaling")
	fPtr := flag.Int("fps", 20, "Frames per second")
	cPtr := flag.Bool("color", true, "Enable color")
	mPtr := flag.Float64("multiplier", 1.2, "Brightness multiplier")
	diPtr := flag.Float64("dither", 0, "Dither intensity")
	iPtr := flag.String("info", "fastfetch --logo-type none", "Info command")
	oPtr := flag.Int("offset", 0, "Top offset")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Println("Usage: brrtfetch [options] /path/to/file.gif")
		return
	}

	gifPath, _ := filepath.Abs(flag.Arg(0))
	baseCfg := Config{FPS: *fPtr, Color: *cPtr, DitherIntensity: *diPtr, Multiplier: *mPtr}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGWINCH)

	// Load GIF first to get its original dimensions for scaling math
	rawGif := loadRawGif(gifPath)
	termW, termH := getTerminalSize()
	currentCfg := resolveDimensions(baseCfg, *wPtr, *hPtr, *sPtr, termW, termH, rawGif.Config.Width, rawGif.Config.Height)

	prerendered := getFrameSequence(rawGif, gifPath, currentCfg)
	sysInfo := getCommandOutputLines(*iPtr)
	sysInfoBytes := toBytes(sysInfo)

	fmt.Print("\033[?1049h" + ANSI_HIDE_CURSOR + ANSI_DISABLE_WRAP)

	writer := bufio.NewWriterSize(os.Stdout, 128*1024)
	var prevFrameLines [][]byte
	ticker := time.NewTicker(time.Second / time.Duration(baseCfg.FPS))
	defer ticker.Stop()

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
				
				// Force clear and re-enable wrap disable on resize
				writer.WriteString("\033[2J\033[H" + ANSI_DISABLE_WRAP)
				
				if newCfg.Width != currentCfg.Width || newCfg.Height != currentCfg.Height {
					currentCfg = newCfg
					writer.Flush()
					sysInfo = getCommandOutputLines(*iPtr)
					sysInfoBytes = toBytes(sysInfo)
					prerendered = getFrameSequence(rawGif, gifPath, currentCfg)
					frameIdx = 0
					prevFrameLines = nil
				}
			} else {
				fmt.Print("\033[?1049l" + ANSI_SHOW_CURSOR + ANSI_ENABLE_WRAP)
				if len(prevFrameLines) > 0 {
					for _, line := range prevFrameLines {
						fmt.Printf("%s\n", line)
					}
				}
				os.Exit(0)
			}

		case <-ticker.C:
			if len(prerendered) == 0 {
				continue
			}

			safeIdx := frameIdx % len(prerendered)
			// Pass termW here so we can truncate lines that are too long
			currentFrameLines := composeFrame(prerendered[safeIdx], sysInfoBytes, *oPtr, currentCfg.Width, termW, termH)

			writer.WriteString(ANSI_HOME)
			maxH := len(currentFrameLines)
			if len(prevFrameLines) > maxH {
				maxH = len(prevFrameLines)
			}

			for y := 0; y < maxH; y++ {
				var currLine []byte
				if y < len(currentFrameLines) {
					currLine = currentFrameLines[y]
				}

				if y < len(prevFrameLines) && bytes.Equal(currLine, prevFrameLines[y]) {
					writer.WriteString(ANSI_CURSOR_DOWN)
				} else {
					if len(currLine) > 0 {
						writer.Write(currLine)
						writer.WriteString(ANSI_CLEAR_LINE)
					} else {
						writer.WriteString("\x1b[0m" + ANSI_CLEAR_LINE)
					}
					writer.WriteString("\r\n")
				}
			}
			writer.Flush()
			prevFrameLines = currentFrameLines
			frameIdx++
		}
	}
}

// --- Helpers ---

func resolveDimensions(base Config, flagW, flagH, flagS, termW, termH, gifW, gifH int) Config {
	cfg := base

	// If user gave absolute dimensions, use them
	if flagW > 0 && flagH > 0 {
		cfg.Width, cfg.Height = flagW, flagH
		return cfg
	}

	// Calculate maximum allowed space based on scale percentage
	maxW := float64(termW) * (float64(flagS) / 100.0)
	maxH := float64(termH) * (float64(flagS) / 100.0)

	// In terminal, 1 character height is ~2 pixels.
	// Aspect ratio is Width / (Height / 2)
	gifRatio := float64(gifW) / (float64(gifH) / 2.0)

	// Try fitting to width first
	w := maxW
	h := w / gifRatio

	// If it overflows the height, fit to height instead
	if h > maxH {
		h = maxH
		w = h * gifRatio
	}

	// Apply flag overrides if only one was provided
	if flagW > 0 {
		cfg.Width = flagW
		cfg.Height = int(float64(cfg.Width) / gifRatio)
	} else if flagH > 0 {
		cfg.Height = flagH
		cfg.Width = int(float64(cfg.Height) * gifRatio)
	} else {
		cfg.Width = int(w)
		cfg.Height = int(h)
	}

	if cfg.Width < 2 {
		cfg.Width = 2
	}
	if cfg.Height < 1 {
		cfg.Height = 1
	}

	return cfg
}

func composeFrame(frameData []byte, sysInfo [][]byte, offset int, gifWidth int, termW, termH int) [][]byte {
	gifLines := bytes.Split(frameData, []byte("\n"))
	totalH := len(gifLines)
	if len(sysInfo)+offset > totalH {
		totalH = len(sysInfo) + offset
	}

	if totalH > termH-1 {
		totalH = termH - 1
	}

	// Calculate how much space we have for the sysinfo text
	// Padding is 3 spaces ("   ")
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
			// Truncate the sysinfo line so it fits without wrapping
			truncated := truncateAnsi(sysInfo[sIdx], allowedTextWidth)
			buf.Write(truncated)
		}

		lineCopy := make([]byte, buf.Len())
		copy(lineCopy, buf.Bytes())
		result[y] = lineCopy
		lineBufferPool.Put(buf)
	}
	return result
}

// truncateAnsi cuts a bytestring to a specific visual width while preserving ANSI codes.
func truncateAnsi(data []byte, maxLen int) []byte {
	if maxLen <= 0 {
		return []byte{}
	}
	var width int
	var inAnsi bool
	end := len(data)

	for i := 0; i < len(data); i++ {
		// Detect start of ANSI escape sequence
		if data[i] == 0x1b {
			inAnsi = true
			continue
		}
		// Detect end of ANSI escape sequence
		if inAnsi {
			// ANSI sequences typically end with letters (m, K, H, etc.)
			if (data[i] >= 'a' && data[i] <= 'z') || (data[i] >= 'A' && data[i] <= 'Z') {
				inAnsi = false
			}
			continue
		}

		// Count visual width (approximation: 1 byte = 1 column, ignoring UTF-8 continuation bytes)
		// 0xC0 (11000000) & byte != 0x80 (10000000) means it's not a continuation byte
		if (data[i] & 0xC0) != 0x80 {
			width++
		}

		if width > maxLen {
			end = i
			break
		}
	}

	// If we truncated, assume we need to reset colors to be safe
	if end < len(data) {
		res := make([]byte, end+4)
		copy(res, data[:end])
		copy(res[end:], []byte("\x1b[0m"))
		return res
	}

	return data
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

func toBytes(lines []string) [][]byte {
	b := make([][]byte, len(lines))
	for i, s := range lines {
		b[i] = []byte(s)
	}
	return b
}

func getTerminalSize() (int, int) {
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	out, err := cmd.Output()
	if err != nil {
		return 80, 24
	}
	parts := strings.Fields(string(out))
	h, _ := strconv.Atoi(parts[0])
	w, _ := strconv.Atoi(parts[1])
	return w, h
}

func processGif(g *gif.GIF, cfg Config) [][]byte {
	numWorkers := runtime.NumCPU()
	jobs := make(chan RenderJob, len(g.Image))
	results := make(chan RenderResult, len(g.Image))
	var wg sync.WaitGroup

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
				renderToBuffer(buf, job.Image, cfg)
				resBytes := make([]byte, buf.Len())
				copy(resBytes, buf.Bytes())
				results <- RenderResult{Index: job.Index, Data: resBytes}
				lineBufferPool.Put(buf)
				imageBufferPool <- job.PoolKey
			}
		}()
	}

	go func() {
		var fullFrame *image.RGBA
		var lastDisposal = gif.DisposalNone
		var lastBounds image.Rectangle
		var snapshot *image.RGBA

		for i, frame := range g.Image {
			if fullFrame == nil {
				fullFrame = image.NewRGBA(image.Rect(0, 0, g.Config.Width, g.Config.Height))
				snapshot = image.NewRGBA(fullFrame.Bounds())
			} else {
				if lastDisposal == gif.DisposalPrevious {
					draw.Draw(fullFrame, fullFrame.Bounds(), snapshot, image.Point{}, draw.Src)
				} else if lastDisposal != gif.DisposalNone {
					draw.Draw(fullFrame, lastBounds, image.NewUniform(color.Transparent), image.Point{}, draw.Src)
				}
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

func renderToBuffer(buf *bytes.Buffer, img *image.RGBA, cfg Config) {
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
				lum1 := 0.21*float64(r1) + 0.72*float64(g1) + 0.07*float64(b1)
				lum2 := 0.21*float64(r2) + 0.72*float64(g2) + 0.07*float64(b2)
				thresh := 100.0 * cfg.Multiplier
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
			} else {
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
		}
		if y < cfg.Height-1 {
			buf.WriteByte('\n')
		}
	}
}

func getCachePath(path string, cfg Config) string {
	info, _ := os.Stat(path)
	hash := md5.Sum([]byte(fmt.Sprintf("%s-%d-%d", path, info.Size(), info.ModTime().UnixNano())))
	return filepath.Join(os.TempDir(), fmt.Sprintf("brrtfetch_%x_%d_%d.bin", hash, cfg.Width, cfg.Height))
}

func loadCache(path string, cfg Config) ([][]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var data CacheData
	gob.NewDecoder(f).Decode(&data)
	if data.Config.DitherIntensity != cfg.DitherIntensity {
		return nil, fmt.Errorf("cfg")
	}
	return data.Frames, nil
}

func saveCache(path string, frames [][]byte, cfg Config) {
	f, _ := os.Create(path)
	defer f.Close()
	gob.NewEncoder(f).Encode(CacheData{Frames: frames, Config: cfg})
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

func getRealShellName() string {
	ppid := os.Getppid()
	cmd := exec.Command("ps", "-p", strconv.Itoa(ppid), "-o", "comm=")
	out, err := cmd.Output()
	if err != nil {
		s := os.Getenv("SHELL")
		if s == "" {
			return "sh"
		}
		return filepath.Base(s)
	}
	return strings.TrimSpace(string(out))
}

func runCommand(cmdLine string) string {
	if cmdLine == "" {
		return ""
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("script", "-q", "/dev/null", "sh", "-c", cmdLine)
	} else {
		cmd = exec.Command("script", "-qec", cmdLine, "/dev/null")
	}
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	out, _ := cmd.CombinedOutput()
	return string(out)
}

func getCommandOutputLines(cmd string) []string {
	out := runCommand(cmd)
	exe, _ := os.Executable()
	binName := filepath.Base(exe)
	shellName := getRealShellName()
	if binName != "" && binName != shellName {
		out = strings.ReplaceAll(out, binName, shellName)
	}
	raw := strings.Split(out, "\n")
	var res []string
	for _, l := range raw {
		l = strings.TrimRight(l, "\r\n")
		if l != "" {
			res = append(res, l)
		}
	}
	return res
}
