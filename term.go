// Low-level terminal control: raw mode via direct darwin ioctls so the
// binary stays dependency-free (no golang.org/x/term).
package main

import (
	"bufio"
	"os"
	"syscall"
	"unsafe"
)

type termios struct {
	Iflag, Oflag, Cflag, Lflag uint64
	Cc                         [20]uint8
	Ispeed, Ospeed             uint64
}

const (
	tiocgeta   = 0x40487413 // TIOCGETA (darwin)
	tiocseta   = 0x80487414 // TIOCSETA (darwin)
	tiocgwinsz = 0x40087468 // TIOCGWINSZ (darwin)
	vmin       = 16
	vtime      = 17
)

func ioctlPtr(fd, req uintptr, arg unsafe.Pointer) error {
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(arg)); e != 0 {
		return e
	}
	return nil
}

func termSize() (rows, cols int) {
	var ws struct{ Rows, Cols, X, Y uint16 }
	if err := ioctlPtr(os.Stdout.Fd(), tiocgwinsz, unsafe.Pointer(&ws)); err == nil && ws.Rows > 0 && ws.Cols > 0 {
		return int(ws.Rows), int(ws.Cols)
	}
	return 24, 80
}

// isTTY reports whether stdin and stdout are an interactive terminal.
func isTTY() bool {
	for _, f := range []*os.File{os.Stdin, os.Stdout} {
		st, err := f.Stat()
		if err != nil || st.Mode()&os.ModeCharDevice == 0 {
			return false
		}
	}
	var t termios
	return ioctlPtr(os.Stdin.Fd(), tiocgeta, unsafe.Pointer(&t)) == nil
}

func enterRaw() (termios, error) {
	var old termios
	if err := ioctlPtr(os.Stdin.Fd(), tiocgeta, unsafe.Pointer(&old)); err != nil {
		return old, err
	}
	raw := old
	raw.Lflag &^= 0x8 | 0x100 | 0x80 | 0x400 // ECHO | ICANON | ISIG | IEXTEN
	raw.Iflag &^= 0x200 | 0x100              // IXON | ICRNL
	raw.Cc[vmin], raw.Cc[vtime] = 1, 0
	return old, ioctlPtr(os.Stdin.Fd(), tiocseta, unsafe.Pointer(&raw))
}

func restoreTerm(old termios) {
	ioctlPtr(os.Stdin.Fd(), tiocseta, unsafe.Pointer(&old))
}

// readKey returns the next keypress, mapping arrow/page keys to their vim
// equivalents ('k','j','l','h', ctrl-u, ctrl-d). A lone ESC returns 27.
func readKey(r *bufio.Reader) (byte, error) {
	c, err := r.ReadByte()
	if err != nil {
		return 0, err
	}
	if c != 27 {
		return c, nil
	}
	if r.Buffered() < 2 {
		return 27, nil
	}
	b1, _ := r.ReadByte()
	if b1 != '[' && b1 != 'O' {
		return 27, nil
	}
	b2, _ := r.ReadByte()
	switch b2 {
	case 'A':
		return 'k', nil
	case 'B':
		return 'j', nil
	case 'C':
		return 'l', nil
	case 'D':
		return 'h', nil
	case '5': // PgUp ~
		r.ReadByte()
		return 21, nil // ctrl-u
	case '6': // PgDn ~
		r.ReadByte()
		return 4, nil // ctrl-d
	}
	return 0, nil
}
