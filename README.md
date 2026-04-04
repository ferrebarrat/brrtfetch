<p align="center">
  <img src="docs/readme-md-brrtfetch-main-textlogo.png" alt="brrtfetch" width="400">
</p>

<p align="center">
  Animated GIF system fetch for your terminal.
</p>

<p align="center">
  <img src="docs/readme-md-main.gif" alt="brrtfetch demo">
</p>

---

brrtfetch renders animated GIFs directly in the terminal alongside live system information. It ships with a built-in interactive fetcher, 13 render modes, an optional embedded shell, frame caching, and responsive resizing.

## Features

- **Animated GIF playback** in any terminal with ANSI support
- **13 render modes** — text-based Unicode, visual effects, and native image protocols
- **Built-in system fetcher** with 4 interactive slides (System, CPU, RAM, Disk)
- **Shell mode** — interactive shell below the animation bar with scrollback
- **Responsive** — adapts to terminal size and handles live resizing
- **Frame caching** — pre-rendered frames cached to `/tmp` for instant restarts
- **Dithering** — Floyd-Steinberg dithering for smoother gradients
- **Mouse support** — click to navigate fetcher slides, scroll shell history

## Installation

### Build from source

Requires Go 1.25+.

```sh
cd go
go build -o brrtfetch .
```

### Nix

```sh
nix build
```

## Usage

```
brrtfetch [options] file.gif
```

### Examples

```sh
# Default: half-block renderer, builtin fetcher, 40% scale
brrtfetch animation.gif

# Shell mode with kitty image protocol
brrtfetch -shell -render kitty animation.gif

# Custom size and speed
brrtfetch -width 60 -fps 30 animation.gif

# Use fastfetch instead of builtin fetcher
brrtfetch -info 'fastfetch --logo-type none' animation.gif

# Benchmark first frame render time
brrtfetch -benchmark animation.gif
```

## Flags

| Flag | Default | Description |
|---|---|---|
| `-width` | `0` | Fixed width in columns. Overrides `-scale` horizontally. `0` = use `-scale` |
| `-height` | `-1` | Fixed height in rows. `-1` = auto from width + aspect ratio. Overrides `-scale` vertically |
| `-scale` | `40` | Scale GIF to this % of terminal size. Ignored per axis when `-width`/`-height` is set |
| `-fps` | `20` | Playback speed in frames per second |
| `-color` | `true` | Color output. `false` for monochrome |
| `-multiplier` | `1.2` | Brightness multiplier for pixel values |
| `-dither` | `0` | Dither intensity. `0` = off, try `0.5`-`1.0` for smoother gradients |
| `-info` | `""` | Info source. Empty = builtin fetcher. Or a command, e.g. `'fastfetch --logo-type none'` |
| `-offset` | `0` | Vertical row offset before info text starts |
| `-render` | `half-block` | Render mode (see below) |
| `-shell` | `false` | Spawn an interactive shell below the animation bar |
| `-benchmark` | `false` | Render one frame, print time taken, and exit |

### Sizing

By default, brrtfetch scales the GIF to 40% of the terminal using `-scale`. You can override this per axis:

- `-width 80` sets a fixed column width; height is derived from the GIF aspect ratio (unless `-height` is also set).
- `-height 20` sets a fixed row height.
- `-width 80 -height 20` sets both explicitly, ignoring aspect ratio.
- `-scale 60` scales to 60% of the terminal on both axes.

When `-width` or `-height` is set, `-scale` is ignored for that axis.

## Render Modes

### Text modes

| Mode | Description |
|---|---|
| `half-block` | Upper/lower half blocks (`▀▄`). Good balance of detail and compatibility. **(default)** |
| `block` | Gradient blocks (`░▒▓█`) with 4x4 sub-pixel pattern matching |
| `quadrant` | Unicode quadrant characters (`▘▝▖▗`) for 2x2 pixel precision |
| `braille` | Braille dots for 2x4 sub-pixel precision. Highest text-mode detail |
| `dots` | Solid circles (`⬤`). Simplest mode |
| `sextant` | Six-dot sextant characters for 2x3 precision |
| `medusa` | ASCII art using brightness-weighted characters (`. * = % # W @`) |

### Effect modes

| Mode | Description |
|---|---|
| `matrix` | Matrix-style directional glyphs (`0 1 { } [ ] / \`) |
| `crt` | Retro CRT scanline effect with phosphor glow |

### Image protocol modes

| Mode | Description |
|---|---|
| `kitty` | [Kitty graphics protocol](https://sw.kovidgoyal.net/kitty/graphics-protocol/). For Kitty terminal |
| `iterm` | [iTerm2 inline images](https://iterm2.com/documentation-images.html). For iTerm2 |
| `wezterm` | WezTerm image protocol. For WezTerm |
| `chafa` | External [chafa](https://hpjansson.org/chafa/) renderer. Requires `chafa` installed |

## Built-in Fetcher

When no `-info` command is specified, brrtfetch displays an interactive system information panel with 4 slides:

| Slide | Info |
|---|---|
| **System** | OS, kernel, arch, hostname, uptime, shell, terminal, DE, WM, packages |
| **CPU** | Model, cores/threads, frequency, usage bar, usage history graph |
| **Memory** | Used/free/buffers/cached, RAM bar, swap bar, usage history graph |
| **Disk** | Per-mount usage bars, free space, total size, filesystem type |

Navigate slides by clicking the `◂` `▸` arrows in the navigation bar. Slides auto-refresh every second.

The system slide adapts to a two-column layout on wider terminals.

## Shell Mode

`-shell` spawns your `$SHELL` below the animation bar with full interactive capabilities:

- Colors, cursor control, alt-screen apps (vim, less, tmux)
- Mouse wheel scrolling through 10,000 lines of scrollback history
- Proper text reflow on terminal resize
- Click-to-navigate fetcher slides in the bar above

<p align="center">
  <img src="docs/readme-md-example-run.gif" alt="brrtfetch shell mode">
</p>

## Platform Support

brrtfetch is built for **Linux**. It reads system information from `/proc` and `/sys`.

## License

[MIT](LICENSE)
