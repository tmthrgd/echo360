package main

import (
	"io"
	"sync"
)

// This is the proposed io.OnceCloser from https://github.com/golang/go/issues/25408.

// ioOnceCloser returns a Closer wrapping c that guarantees it only calls c.Close
// once and is safe for use by multiple goroutines. Each call to the returned Closer
// will return the same value, as returned by c.Close.
func ioOnceCloser(c io.Closer) io.Closer {
	return &onceCloser{c: c}
}

type onceCloser struct {
	c    io.Closer
	once sync.Once
	err  error
}

func (c *onceCloser) Close() error {
	c.once.Do(c.close)
	return c.err
}

func (c *onceCloser) close() {
	c.err = c.c.Close()
}
