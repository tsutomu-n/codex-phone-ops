//go:build linux

package control

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"unicode"
	"unsafe"
)

// A small single-process terminal selector. No PTY or WebSocket implementation
// is embedded. During native handoff all terminal ownership returns to ssh.
type Terminal struct {
	mu    sync.Mutex
	saved syscall.Termios
	raw   bool
	Color bool
}

func NewTerminal() (*Terminal, error) {
	t := &Terminal{Color: os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"}
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, os.Stdin.Fd(), syscall.TCGETS, uintptr(unsafe.Pointer(&t.saved))); e != 0 {
		return nil, errors.New("対話ターミナルで実行してください（パイプ入力は非対応）")
	}
	return t, nil
}
func (t *Terminal) Enter() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	raw := t.saved
	raw.Lflag &^= syscall.ICANON | syscall.ECHO
	raw.Iflag &^= syscall.ICRNL | syscall.IXON
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, os.Stdin.Fd(), syscall.TCSETS, uintptr(unsafe.Pointer(&raw))); e != 0 {
		return e
	}
	t.raw = true
	fmt.Print("\x1b[?1049h\x1b[?25l")
	return nil
}
func (t *Terminal) Leave() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.raw {
		syscall.Syscall(syscall.SYS_IOCTL, os.Stdin.Fd(), syscall.TCSETS, uintptr(unsafe.Pointer(&t.saved)))
		t.raw = false
	}
	fmt.Print("\x1b[0m\x1b[?25h\x1b[?1049l")
}
func (t *Terminal) ResetLine() { fmt.Print("\x1b[0m\x1b[?25h") }
func (t *Terminal) Size() (int, int) {
	w := struct{ Row, Col, X, Y uint16 }{}
	syscall.Syscall(syscall.SYS_IOCTL, os.Stdin.Fd(), syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&w)))
	cols, rows := int(w.Col), int(w.Row)
	if cols < 20 {
		cols = 40
	}
	if rows < 8 {
		rows = 24
	}
	return cols, rows
}
func readyInput() bool {
	var fds syscall.FdSet
	fds.Bits[0] = 1
	tv := syscall.Timeval{Usec: 70000}
	n, e := syscall.Select(1, &fds, nil, nil, &tv)
	return e == nil && n > 0
}
func (t *Terminal) Key() (string, error) {
	var b [1]byte
	if _, e := io.ReadFull(os.Stdin, b[:]); e != nil {
		return "", e
	}
	if b[0] == 27 {
		if !readyInput() {
			return "esc", nil
		}
		io.ReadFull(os.Stdin, b[:])
		if b[0] == '[' || b[0] == 'O' {
			if !readyInput() {
				return "esc", nil
			}
			io.ReadFull(os.Stdin, b[:])
			switch b[0] {
			case 'A':
				return "up", nil
			case 'B':
				return "down", nil
			case 'C':
				return "right", nil
			case 'D':
				return "left", nil
			case '5', '6':
				ch := b[0]
				if readyInput() {
					io.ReadFull(os.Stdin, b[:])
				}
				if ch == '5' {
					return "pageup", nil
				}
				return "pagedown", nil
			}
		}
		return "esc", nil
	}
	if b[0] == 13 || b[0] == 10 {
		return "enter", nil
	}
	if b[0] == 127 {
		return "backspace", nil
	}
	return string(b[:]), nil
}
func (t *Terminal) Paint(title string, lines []string, footer string) {
	w, h := t.Size()
	fmt.Print("\x1b[H\x1b[2J")
	if t.Color {
		fmt.Print("\x1b[1;36m")
	}
	fmt.Println(Clip(title, w))
	if t.Color {
		fmt.Print("\x1b[0m")
	}
	fmt.Println(strings.Repeat("─", w))
	cap := h - 4
	for i := 0; i < cap; i++ {
		if i < len(lines) {
			s := lines[i]
			if t.Color && strings.HasPrefix(s, "> ") {
				fmt.Print("\x1b[1;30;46m")
			}
			fmt.Print(Clip(s, w))
			if t.Color {
				fmt.Print("\x1b[0m")
			}
		}
		fmt.Print("\n")
	}
	if t.Color {
		fmt.Print("\x1b[2m")
	}
	fmt.Print(Clip(footer, w))
	if t.Color {
		fmt.Print("\x1b[0m")
	}
}

var csi = regexp.MustCompile("\x1b\\[[0-?]*[ -/]*[@-~]")
var osc = regexp.MustCompile("\x1b\\][^\x07\x1b]*(\x07|\x1b\\\\)")

func Safe(s string) string {
	s = osc.ReplaceAllString(s, "")
	s = csi.ReplaceAllString(s, "")
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return -1
		}
		return r
	}, s)
}
func runeWidth(r rune) int {
	if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) {
		return 0
	}
	if r >= 0x1100 && (r <= 0x115f || r == 0x2329 || r == 0x232a || (r >= 0x2e80 && r <= 0xa4cf) || (r >= 0xac00 && r <= 0xd7a3) || (r >= 0xf900 && r <= 0xfaff) || (r >= 0xfe10 && r <= 0xfe6f) || (r >= 0xff00 && r <= 0xff60) || (r >= 0xffe0 && r <= 0xffe6) || (r >= 0x1f300 && r <= 0x1faff) || r >= 0x20000) {
		return 2
	}
	return 1
}
func Width(s string) int {
	n := 0
	for _, r := range Safe(s) {
		n += runeWidth(r)
	}
	return n
}
func Clip(s string, w int) string {
	s = Safe(s)
	if Width(s) <= w {
		return s
	}
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n+runeWidth(r) > w-1 {
			break
		}
		b.WriteRune(r)
		n += runeWidth(r)
	}
	return b.String() + "…"
}
func Wrap(s string, w int) []string {
	out := []string{}
	for _, line := range strings.Split(s, "\n") {
		line = Safe(line)
		var b strings.Builder
		n := 0
		for _, r := range line {
			rw := runeWidth(r)
			if n+rw > w && n > 0 {
				out = append(out, b.String())
				b.Reset()
				n = 0
			}
			b.WriteRune(r)
			n += rw
		}
		out = append(out, b.String())
	}
	return out
}

// Restore also handles a child program that changed termios in canonical mode.
func (t *Terminal) Restore() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, os.Stdin.Fd(), syscall.TCSETS, uintptr(unsafe.Pointer(&t.saved)))
	fmt.Print("\x1b[0m\x1b[?25h\x1b[?1049l")
	if e != 0 {
		return fmt.Errorf("端末復元に失敗しました。再送せず終了します: %w", e)
	}
	t.raw = false
	return nil
}
