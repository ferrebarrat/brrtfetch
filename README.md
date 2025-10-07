<h3 align="center"><img src="./docs/readme-md-brrtfetch-main-textlogo.png" alt="logo" height="100px"></h3>
<p align="center"><img src="./docs/readme-md-main.gif" height="400px"></p>
<p align="center"><i>Fastfetch config: https://github.com/xerolinux/xero-layan-git</i></p>

**Brrtfetch** is an animated system information fetcher written mainly in Go. Please keep in mind that it is still in it's very early stage of development. It displays the user specified **GIF rendered as animated ASCII art** alongside the system information from your favourite fetcher.

Think of it like a renderer that replaces the ASCII art for your fetcher with **animated** art. You only need to provide a .gif file.

Broken on MacOS. It does not display sysinfo. I do not own any Apple devices so apologies if this takes a while to fix.

In the next version I plan to use a new way of deciding what to put where in the terminal. It should be way more stable. No more color bugs, already confirmed hyfetch works(no workaround needed),the issue with illegal flags for the script binary on MacOS will also be resolved in the upcoming release. You can also expect a very cool new feature which I have not seen on any other animated fetchers when the new update is released.

---
 
## ✨ Features

* Render animated GIFs as **colorful ASCII art** directly in your terminal.
* Side-by-side system information via `fastfetch`, `neofetch`, or your fetcher of choice. I have only tested with `fastfetch`, `neofetch` and `hyfetch`. Hyfetch requires a small workaround and even then it's still a bit buggy with hyfetch. See examples below. 
* **True color (24-bit ANSI)** support with optional white monochrome mode via `-color=false`.
* **Multithreaded prerendering** for smooth playback.
* Configurable:

  * Width / height to render at
  * FPS to render at (impacts animation speed)
  * Brightness multiplier (controls density of ASCII mapping)
  * Vertical offset for aligning sysinfo height relative to  ASCII art
* Attempts to preserves **ANSI color codes** from sysinfo commands (broken for hyfetch and Windows CMD/Powershell. WSL does show color for the sysinfo. Only tested this with Ubuntu for WSL).
* If you can somehow render DOOM in GIF format you could technically use this to play DOOM in your fetcher. It would only be (re)rendered in brrtfetch, not actually run inside of it, at least for now ;)

---

## 📦 Installation

More comprehensive instructions for different distros and support for various package managers will be coming soon.

Debian/Ubuntu based steps only for the initial release, it should work on any linux system as long as you replace apt with your package manager for the dependencies. You can install it on Windows and Mac if you want. Just translate the steps to Windows. Will try to add Winget support later so i don't have to make an install script/instructions for Powershell. I will also attempt to add support for all major Linux package managers and Brew.


### Prerequisites

* A terminal that supports ANSI colors and escape sequences. Almost all modern terminals do.
* `Script` (Linux only) 

  Optional but highly recommended for sysinfo color support. Part of the **bsdutils** package. Comes by default on most systems. Check with "which script"
* `Unbuffer` (Linux only)

  Optional but recommended. Part of the `expect` package. Install with "apt install expect" or any other package manager. Brrtfetch will attempt to fallback on `unbuffer` if `script` is not available. 
* A fetch application with an option to omit the ASCII art.

  * [fastfetch](https://github.com/fastfetch-cli/fastfetch) (default)
  * [hyfetch](https://github.com/hykilpikonna/hyfetch)
  * Or any command you like, it can be specified with `-info "neofetch --off"` or even `-info "echo $USER"` or anything custom if you want.

  ```bash
  apt install fastfetch # only works on Debian 13+, see fastfetch docs for other version and distros
  apt install bsdutils expect
  ```

### Build from source

Additional prerequisite:
* Go 1.20+ (I used Go 1.23.3, will assume 1.20+ works)

  ```bash
  # Install Go (replace apt with your package manager like brew, yum, pacman etc)
  sudo apt install golang

  # Build
  git clone https://github.com/ferrebarrat/brrtfetch
  cd brrtfetch 
  go build -o ./bin/brrtfetch ./go/main.go && chmod +x ./bin/brrtfetch

  # Add to path
  sudo cp ./bin/brrtfetch /usr/local/bin/brrtfetch

  # Optional - Save gifs from repo before cleanup
  mkdir -p /home/$USER/Pictures/brrtfetch/gifs
  cp -r ./gifs/* /home/$USER/Pictures/brrtfetch/gifs

  # Cleanup
  cd .. && rm -rf brrtfetch
  ```

---

## 🎮 Usage

  ```bash
  brrtfetch [options] /path/to/file.gif
  ```

* **Ctrl-C** → attempts to exit the animation gracefully, clears and restores terminal, prints first frame with sysinfo and returns you to your prompt as if it was just a static fetcher.
* Animation loops endlessly until interrupted with **CTRL-C**.

<p><img src="./docs/readme-md-example-run.gif" height="300px"></p>

---

## ⚙️ Options

| Flag          | Default                        | Description                                                           |
| ------------- | ------------------------------ | --------------------------------------------------------------------- |
| `-width`      | `40`                           | Width of ASCII animation (columns)                                    |
| `-height`     | `width`                        | Height of ASCII animation (rows)                                      |
| `-fps`        | `17`                           | Frames per second for playback                                        |
| `-multiplier` | `1.2`                          | Brightness multiplier (higher = denser, lower = more transparency)    |
| `-color`      | `true`                         | Enable color output (true = 24-bit ANSI, false = monochrome)          |
| `-info`       | `"fastfetch --logo-type none"` | Command to run for system info (omit ASCII logos!)                    |
| `-offset`     | `0`                            | Number of empty lines before sysinfo output (shifts sysinfo downward) |

---

## 🧩 Examples

* Default (fastfetch):

  ```bash
  brrtfetch /path/to/file.gif

  # Same as:

  brrtfetch -info "fastfetch --logo-type none" /path/to/file.gif
  ```

* Command to replicate GIF at the top of this readme
  ```bash
  brrtfetch -width 80 -fps 17 -multiplier 10 -offset 3 /home/$USER/Pictures/brrtfetch/gifs/defaults/brrt.gif
  ```

* Fastfetch with custom config

  ```bash
  brrtfetch -width 60 -height 60 -fps 20 -color=true -info "fastfetch --logo-type none --config hypr" /home/$USER/Pictures/brrtfetch/gifs/defaults/brrt.gif
  ```

  This will render debian.gif as animated ASCII alongside the sysinfo from `fastfetch`

* Hyfetch: Only works with this `echo` workaround for now. It's still a bit buggy, first line is inside art and no sysinfo colors at the moment. 

  You also **need a file that contains just a single space**, the example uses `hyfetch_single_space.txt` for that. This file with just a single space would be required even without the workaround since it is needed to omit the ASCII art from hyfetch, empty or non-existent files won't work.

  ```bash
  brrtfetch -info "echo \"$(hyfetch --ascii-file=hyfetch_single_space.txt)\"" /home/$USER/Pictures/brrtfetch/gifs/random/torvalds.gif
  ```

* Neofetch

  Please note that the --off flag for `neofetch` does not work on Windows. In Windows you will need to play with the `neofetch` config to omit the art.

  ```bash
  brrtfetch -width 60 -info "neofetch --off" /home/$USER/Pictures/brrtfetch/gifs/pokemon/gengar.gif
  ```

* Screenfetch

  ```bash
  brrtfetch -width 40 -height 40 -info "screenfetch -n" /home/$USER/Pictures/brrtfetch/gifs/pokemon/magikarp.gif
  ```


---

## 📝 Notes

* Brrtfetch will try to preserve ANSI color output for the sysinfo from your fetcher.

  * Uses `script` if available (best for color preservation).
  * Falls back to `unbuffer`.
  * Otherwise runs the command specified with `-info` normally without `script` or `unbuffer`. 

---

## ⚠️ Technical limitations

* You have to CTRL-C to exit the animation before being able to use your terminal.
* The animation will stop after you CTRL-C.
* The sysinfo does not have color support on Windows except for WSL.
* Loss of detail: The width and height flags form a sort of virtual screen. You can pretend each ASCII character is a virtual pixel. Ask yourself the question, would this GIF look good on a screen of 50px by 50px. If yes it will also look good in brrtfetch if you set the width and height to 50.
* For some systems the animated GIFs can appear a bit stretched. You can fix this by playing with the `-width` and `-height` flags. This probably has something to do with spacing between your individual ASCII characters beings smaller then most systems. I only encountered this on my Arch/Hyprland machine. This is not a bug in brrtfetch.
* Increasing the value of the `-fps` flag will increase the speed of the animation and vice versa for decreasing.
* Does not auto detect distro. If you don't specify a GIF it will complain for now. Might add OS/distro detection after i have some nice GIFs for all major distro logo's. 

## 🧪 Tested on
| OS / Distro                    | Sysinfo fetcher                        | Notes                                                                                                                                                    |
| ------------------------------ | -------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| EndevourOS (Arch Linux)        | `Fastfetch`, `Neofetch`, `Screenfetch` | No major issues, this is the current baseline to which i will compare the rest to.                                                                       |
| Ubuntu (WSL)                   | `Fastfetch`, `Neofetch`, `Screenfetch` | No major issues.                                                                                                                                         |
| Windows 11                     | `Fastfetch`                            | No colors for sysinfo. `Neofetch --off` does not work on Windows, you will have to disable the art via the conf files for `neofetch` on Windows.         |
| \<Any>                         | `Hyfetch`                              | Workaround required (see examples) or it won't show any sysinfo at all. First line (usually hostname) is inside of the ASCII art. No colors for sysinfo. |

---

## 👾 Create your own animated GIFs from scratch (external tools)

Disclaimer: You are free (and strongly encouraged) to find alternatives to Meta for animating the static image in the steps below. It's the best I could find that allows you to generate a bunch of animations for 'free'. If you have the processing power and don't mind getting your hands dirty you could setup your own Open-Sora: https://github.com/hpcaitech/Open-Sora. 

1. Use your preferred AI to generate a static image of something you want to animate. If you want it to have a transparent background in your terminal later, ask it to use a "greenscreen type green" background. This is important for later. 
2. Open Meta Vibes in your browser, login and go to the create tab. Now change from image to video, add your static image generated in step 1 and provide the prompt that tells Meta how to animate it.
3. Download the .mp4 and convert it to a GIF with your preferred online or CLI tool.
4. Upload the GIF to https://onlinegiftools.com/create-transparent-gif. Select your background color (eg green, white, black, ... ) under 'Tool Options' -> 'Transparent Regions' and play with the percentage input under it until it looks like a GIF with a transparent background.  
5. You can now click 'Save as...' to download your final GIF that is now ready to use in brrtfetch

The octopus at the very top of this readme was generated using this exact method.


Looking forward to seeing your creations on 
