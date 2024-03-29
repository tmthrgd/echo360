package main

import (
	"io"
	"os"
	"strings"
	"sync"

	"github.com/fatih/color"
)

var (
	fatalColor  = color.New(color.FgRed, color.Bold)
	errorColor  = color.New(color.FgRed)
	noticeColor = color.New(color.FgBlue, color.Bold)
	infoColor   = color.New(color.FgYellow)
)

func trailingNL(s string) string {
	if strings.HasSuffix(s, "\n") {
		return s
	}

	return s + "\n"
}

func logInfo(format string, args ...interface{}) {
	logDo(func() {
		infoColor.Fprintf(os.Stderr, trailingNL(format), args...)
	})
}

func logNotice(format string, args ...interface{}) {
	logDo(func() {
		noticeColor.Fprintf(os.Stderr, trailingNL(format), args...)
	})
}

func logError(format string, args ...interface{}) {
	logDo(func() {
		errorColor.Fprintf(os.Stderr, trailingNL(format), args...)
	})
}

func logFatal(format string, args ...interface{}) {
	logDo(func() {
		fatalColor.Fprintf(os.Stderr, trailingNL(format), args...)
		os.Exit(1)
	})
}

var logMu sync.Mutex

func logDo(fn func()) {
	logMu.Lock()
	defer logMu.Unlock()

	if !progressStarted {
		fn()
		return
	}

	out := progress.Out
	defer progress.SetOut(out)

	// We use the address of this array as a sentinel.
	var dummy [1]byte

	progress.SetOut(writerFn(func(p []byte) (int, error) {
		if len(p) == 0 || &p[0] != &dummy[0] {
			// We need to forward all other writes before
			// and after we hold the progress lock.
			return out.Write(p)
		}

		// Clear the current line with an ANSI escape sequence.
		io.WriteString(os.Stderr, "\x1b[2K\r")

		fn()
		return len(p), nil
	}))

	// The bypass writer holds a lock and clears the line count
	// during Write calls. By abusing this, we can dance around
	// and ensure that our log outputs won't be clobbered by the
	// progress bars.
	progress.Bypass().Write(dummy[:])
}

type writerFn func([]byte) (int, error)

func (fn writerFn) Write(p []byte) (int, error) {
	return fn(p)
}
