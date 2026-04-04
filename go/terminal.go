package main

import (
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "runtime"
    "strconv"
    "strings"
    "syscall"
    "unsafe"
)

const (
    ANSI_HIDE_CURSOR    = "\033[?25l"
    ANSI_SHOW_CURSOR    = "\033[?25h"
    ANSI_HOME           = "\033[H"
    ANSI_CURSOR_DOWN    = "\033[1B"
    ANSI_DISABLE_WRAP   = "\033[?7l"
    ANSI_ENABLE_WRAP    = "\033[?7h"
    ANSI_CLEAR_LINE     = "\x1b[K"
    ANSI_SAVE_CURSOR    = "\033[s"
    ANSI_RESTORE_CURSOR = "\033[u"
    ANSI_RESET_SCROLL   = "\033[r"
    ANSI_MOUSE_ON       = "\033[?1000h\033[?1006h"
    ANSI_MOUSE_OFF      = "\033[?1000l\033[?1006l"
)

func SetScrollRegion(top, bottom int) string {
    return fmt.Sprintf("\033[%d;%dr", top, bottom)
}

func MoveCursor(row, col int) string {
    return fmt.Sprintf("\033[%d;%dH", row, col)
}

func getTerminalSize() (int, int) {
    var ws struct {
        Row    uint16
        Col    uint16
        Xpixel uint16
        Ypixel uint16
    }
    for _, fd := range []uintptr{uintptr(syscall.Stdout), uintptr(syscall.Stdin), uintptr(syscall.Stderr)} {
        _, _, err := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws)))
        if err == 0 && ws.Col > 0 && ws.Row > 0 {
            return int(ws.Col), int(ws.Row)
        }
    }
    return 80, 24
}

func truncateAnsi(data []byte, maxLen int) []byte {
    if maxLen <= 0 { return []byte{} }
    var width int
    var inAnsi bool
    end := len(data)

    for i := 0; i < len(data); i++ {
        if data[i] == 0x1b {
            inAnsi = true
            continue
        }
        if inAnsi {
            if (data[i] >= 'a' && data[i] <= 'z') || (data[i] >= 'A' && data[i] <= 'Z') {
                inAnsi = false
            }
            continue
        }
        if (data[i] & 0xC0) != 0x80 { width++ }
        if width > maxLen {
            end = i
            break
        }
    }
    if end < len(data) {
        res := make([]byte, end+4)
        copy(res, data[:end])
        copy(res[end:], []byte("\x1b[0m"))
        return res
    }
    return data
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
        if l != "" { res = append(res, l) }
    }
    return res
}

func runCommand(cmdLine string) string {
    if cmdLine == "" { return "" }
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

func getRealShellName() string {
    ppid := os.Getppid()
    cmd := exec.Command("ps", "-p", strconv.Itoa(ppid), "-o", "comm=")
    out, err := cmd.Output()
    if err != nil {
        if s := os.Getenv("SHELL"); s != "" { return filepath.Base(s) }
        return "sh"
    }
    return strings.TrimSpace(string(out))
}
