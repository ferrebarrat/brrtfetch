package main

import (
	"bufio"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Config struct to hold CLI overrides or defaults
type Config struct {
	Width  int
	Height int
	FPS    int
	Color  bool
}

// Job represents a frame to be rendered concurrently
type RenderJob struct {
	Index   int
	Image   *image.RGBA
	PoolKey *image.RGBA // Key to return the buffer to the pool
}

// Result holds the prerendered ASCII strings and their index
type RenderResult struct {
	Index int
	Lines []string
}

// Global channel for recycling image buffers (the pool)
var bufferPool chan *image.RGBA

// ANSI escape codes for cursor control
const (
	ANSI_HIDE_CURSOR = "\033[?25l"
	ANSI_SHOW_CURSOR = "\033[?25h"
)

func main() {
	// --- Flags ---
	width := flag.Int("width", 40, "Width of ASCII animation (in chars)")
	height := flag.Int("height", -1, "Height of ASCII animation (in chars)")
	fps := flag.Int("fps", 17, "Frames per second for playback, more fps = faster animation")
	multiplier := flag.Float64("multiplier", 1.2, "Multiplier for ASCII char determination. Higher = denser, lower means whites could be displayed as transparent in some cases")
	colorOutput := flag.Bool("color", true, "Disable color for animated art with -color=false (true = 24-bit ANSI, false = monochrome)")
	infoCommand := flag.String("info", "fastfetch --logo-type none", "Command to execute for system information output, make sure you omit the art. By default it will attempt to use 'fastfetch --logo-type none'")
	offset := flag.Int("offset", 0, "Number of empty lines before sysinfo output")
	flag.Parse()

	// If height wasn't set, sync it to width
	if *height == -1 {
		*height = *width
	}

    *height = *height / 2

	if flag.NArg() < 1 {
		fmt.Println("Usage: gofetch [options] /path/to/file.gif")
		flag.PrintDefaults()
		return
	}

	gifPath := flag.Arg(0)
	f, err := os.Open(gifPath)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	g, err := gif.DecodeAll(f)
	if err != nil {
		panic(err)
	}

	// EXECUTE EXTERNAL INFO COMMAND
	sysInfo := getCommandOutputLines(*infoCommand)

	// --- Build cfg from flags ---
	cfg := Config{
		Width:  *width,
		Height: *height,
		FPS:    *fps,
		Color:  *colorOutput,
	}

	// --- Enter alternate screen buffer ---
	fmt.Print("\033[?1049h")
	defer func() {
		fmt.Print("\033[?1049l") // Exit alternate screen on program exit
	}()

	// --- Setup cursor visibility ---
	writer := bufio.NewWriter(os.Stdout)
	defer func() {
		writer.WriteString(ANSI_SHOW_CURSOR)
		writer.Flush()
	}()
	writer.WriteString(ANSI_HIDE_CURSOR)
	writer.Flush()

	// --- Handle Ctrl-C gracefully ---
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// === CONCURRENT PRERENDERING SETUP ===
	numWorkers := runtime.NumCPU()
	jobs := make(chan RenderJob, len(g.Image))
	results := make(chan RenderResult, len(g.Image))
	var wg sync.WaitGroup

	// 1. Initialize Buffer Pool
	bufferPool = make(chan *image.RGBA, numWorkers*2)
	for i := 0; i < cap(bufferPool); i++ {
		bufferPool <- image.NewRGBA(image.Rect(0, 0, g.Config.Width, g.Config.Height))
	}

	// 2. Start worker goroutines
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go worker(w, jobs, results, cfg, sysInfo, &wg, *multiplier, *offset)
	}

	// 3. Composing and dispatching jobs (handling GIF disposal methods)
	var fullFrame *image.RGBA
	var lastDisposal = gif.DisposalNone
	var lastBounds image.Rectangle
	var snapshot *image.RGBA

	for i, frame := range g.Image {
		if fullFrame == nil {
			fullFrame = image.NewRGBA(image.Rect(0, 0, g.Config.Width, g.Config.Height))
			snapshot = image.NewRGBA(fullFrame.Bounds())
			draw.Draw(fullFrame, fullFrame.Bounds(), image.NewUniform(color.Transparent), image.Point{}, draw.Src)
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

		frameCopy := <-bufferPool
		copy(frameCopy.Pix, fullFrame.Pix)
		jobs <- RenderJob{Index: i, Image: frameCopy, PoolKey: frameCopy}
	}
	close(jobs)

	// 4. Collect results
	prerendered := make([][]string, len(g.Image))
	go func() {
		for result := range results {
			prerendered[result.Index] = result.Lines
		}
	}()

	// 5. Wait for workers and close results
	wg.Wait()
	close(results)

	// --- Capture first frame for printing after Ctrl-C ---
	firstFrame := prerendered[0]
	go func() {
		<-sigs
		fmt.Print("\033[?1049l") // exit alternate screen
		for _, line := range firstFrame {
			fmt.Println(line)
		}
		fmt.Print(ANSI_SHOW_CURSOR)
		fmt.Print("\033[0m")
		os.Exit(0)
	}()

	delay := time.Duration(1000/cfg.FPS) * time.Millisecond

	// ----- Animation loop -----
	for {
		for _, frameStrings := range prerendered {
			writer.WriteString("\033[H")        // Home cursor
			for _, line := range frameStrings { // print all lines returned by renderFrame
				writer.WriteString(line)
				writer.WriteByte('\n')
			}
			writer.Flush()
			time.Sleep(delay)
		}
	}
}

// worker goroutine function
func worker(id int, jobs <-chan RenderJob, results chan<- RenderResult,
	cfg Config, sysInfo []string, wg *sync.WaitGroup, multiplier float64, offset int) {
	defer wg.Done()
	for job := range jobs {
		lines := renderFrame(job.Image, cfg.Width, cfg.Height, sysInfo, cfg.Color, multiplier, offset)
		results <- RenderResult{Index: job.Index, Lines: lines}
		bufferPool <- job.PoolKey
	}
}

// Convert a frame to ASCII lines
func renderFrame(img *image.RGBA, width, height int, sysInfo []string, colorOutput bool, multiplier float64, offset int) []string {
	// totalHeight ensures we can print all sysinfo lines
	totalHeight := height
	if len(sysInfo)+offset > height {
		totalHeight = len(sysInfo) + offset
	}

	lines := make([]string, totalHeight)
	pix := img.Pix
	stride := img.Stride
	scaleX := float64(img.Bounds().Dx()) / float64(width)
	scaleY := float64(img.Bounds().Dy()) / float64(height)
	var lineBuilder strings.Builder

	for y := 0; y < totalHeight; y++ {
		lineBuilder.Reset()

		// Fill GIF lines
		if y < height {
			for x := 0; x < width; x++ {
				px := int(float64(x) * scaleX)
				py := int(float64(y) * scaleY)
				offsetPix := py*stride + px*4
				r8, g8, b8, a8 := pix[offsetPix], pix[offsetPix+1], pix[offsetPix+2], pix[offsetPix+3]

				if a8 == 0 {
					lineBuilder.WriteString("\x1b[0m ")
				} else {
					char := pixelToASCII(r8, g8, b8, multiplier)
					if colorOutput {
						lineBuilder.WriteString(fmt.Sprintf("\x1b[38;2;%d;%d;%dm%s\x1b[0m", r8, g8, b8, char))
					} else {
						lineBuilder.WriteString(char)
					}
				}
			}
		} else {
			// Pad with spaces if GIF is shorter than totalHeight
			lineBuilder.WriteString(strings.Repeat(" ", width))
		}

		// Append sysinfo line if exists and within offset
		sysIndex := y - offset
		if sysIndex >= 0 && sysIndex < len(sysInfo) {
			lineBuilder.WriteString("   ")
			lineBuilder.WriteString(sysInfo[sysIndex])
		}

		lines[y] = lineBuilder.String()
	}

	return lines
}

// Map pixel brightness to ASCII
func pixelToASCII(r, g, b uint8, multiplier float64) string {
	lum := 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
	switch {
	case lum > 1000*multiplier: // Needs retuning
		return " "
	case lum > 250*multiplier:
		return "."
	case lum > 180*multiplier:
		return "◌"
	case lum > 140*multiplier:
		return "*"
	case lum > 120*multiplier:
		return "●"
	case lum > 60*multiplier:
		return "⦾"
	case lum > 30*multiplier:
		return "⦿"
	default:
		return "⬤"
	}
}

// replace your existing runCommand with this one
func runCommand(commandLine string) string {
	parts := strings.Fields(commandLine)
	if len(parts) == 0 {
		return ""
	}

	run := func(name string, args ...string) (string, error) {
		cmd := exec.Command(name, args...)
		cmd.Env = append(os.Environ(), "TERM=xterm-256color")
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	// 1) Try `script` with safe flags
	if _, err := exec.LookPath("script"); err == nil {
		// -q quiet, -e exit immediately, -f flush, -c to run command, /dev/null as log
		out, _ := run("script", "-qefc", commandLine+" 2>/dev/null", "/dev/null")
		return out
	}

	// 2) Try unbuffer
	if _, err := exec.LookPath("unbuffer"); err == nil {
		out, _ := run("unbuffer", parts...)
		return out
	}

	// 3) Fallback
	out, err := run(parts[0], parts[1:]...)
	if err != nil {
		return fmt.Sprintf("Error running command: %s\n%s", parts[0], out)
	}
	return out
}

// getCommandOutputLines executes the command and returns trimmed lines
func getCommandOutputLines(commandLine string) []string {
	output := runCommand(commandLine)
	lines := strings.Split(output, "\n")
	var cleanLines []string
	for _, line := range lines {
		// Trim trailing CR/LF
		line = strings.TrimRight(line, "\r\n")
		if line != "" {
			cleanLines = append(cleanLines, line)
		}
	}
	return cleanLines
}
