package main

import (
	"os"
	"strings"

	"github.com/fatih/color"
)

var (
	fatalColor  = color.New(color.FgRed, color.Bold)
	errorColor  = color.New(color.FgRed)
	noticeColor = color.New(color.FgBlue)
	infoColor   = color.New(color.FgYellow)
)

func trailingNL(s string) string {
	if strings.HasSuffix(s, "\n") {
		return s
	}

	return s + "\n"
}

func logInfo(format string, args ...interface{}) {
	infoColor.Fprintf(os.Stderr, trailingNL(format), args...)
}

func logNotice(format string, args ...interface{}) {
	noticeColor.Fprintf(os.Stderr, trailingNL(format), args...)
}

func logError(format string, args ...interface{}) {
	errorColor.Fprintf(os.Stderr, trailingNL(format), args...)
}

func logFatal(format string, args ...interface{}) {
	fatalColor.Fprintf(os.Stderr, trailingNL(format), args...)
	os.Exit(1)
}
