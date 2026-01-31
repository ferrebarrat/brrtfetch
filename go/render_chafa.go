package main

import (
	"bytes"
	"image"
	"image/png"
	"os/exec"
	"strconv"
)

type ChafaRenderer struct{}

func (c *ChafaRenderer) Render(buf *bytes.Buffer, img *image.RGBA, cfg Config) {
	// Use "-" instead of "png:-" for maximum compatibility
	// Explicitly setting symbols ensures we get the text-based output
	args := []string{
		"--size", strconv.Itoa(cfg.Width) + "x" + strconv.Itoa(cfg.Height),
		"--format", "symbols", 
		"-", 
	}

	if !cfg.Color {
		args = append(args, "--color-mode", "bw")
	}

	cmd := exec.Command("chafa", args...)

	// Setup pipes
	stdin, _ := cmd.StdinPipe()
	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	if err := cmd.Start(); err != nil {
		buf.WriteString("[Chafa Start Error]")
		return
	}

	// Encode as PNG to stdin
	go func() {
		defer stdin.Close()
		// PNG is the safest format to pipe to Chafa
		_ = png.Encode(stdin, img)
	}()

	if err := cmd.Wait(); err != nil {
		// Catching the actual error message again if this fails
		buf.WriteString("[Chafa Error: " + errBuf.String() + "]")
		return
	}

	buf.Write(outBuf.Bytes())
}
