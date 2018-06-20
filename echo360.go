package main

import (
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/MercuryEngineering/CookieMonster"
	"github.com/gosuri/uiprogress"
)

func init() {
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "Usage of %s:\n", filepath.Base(os.Args[0]))
		fmt.Fprintf(out, "%s [flags] <url> \n", os.Args[0])
		flag.PrintDefaults()
	}
}

func main() {
	threads := flag.Int("threads", runtime.GOMAXPROCS(0), "the number of downloads to run in parallel")
	cookiesPath := flag.String("cookies", "cookies.txt", "the path to a Netscape cookies file")
	dir := flag.String("out", "$PWD", "the output path")
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}

	var hasOut bool
	flag.Visit(func(flag *flag.Flag) {
		hasOut = hasOut || flag.Name == "out"
	})

	if !hasOut {
		var err error
		if *dir, err = os.Getwd(); err != nil {
			logFatal("echo360: failed to get working directory: %v", err)
		}
	}

	u, err := url.Parse(flag.Arg(0))
	if err != nil {
		logFatal("echo360: failed to parse url: %v", err)
	}

	switch u.Scheme {
	case "https":
	case "http":
		u.Scheme = "https"
	default:
		logFatal("echo360: unsupported scheme %q", u.Scheme)
	}

	if u.Host != "echo360.org.au" {
		logFatal("echo360: unsupported host %q", u.Host)
	}

	if !strings.HasPrefix(u.Path, "/section/") {
		logFatal("echo360: unsupported path %q", u.Path)
	}

	parts := strings.Split(u.Path, "/")
	if len(parts) < 3 {
		logFatal("echo360: unsupported path %q", u.Path)
	}

	if *cookiesPath == "" {
		logFatal("echo360: -cookies flag cannot be empty")
	}

	cookies, err := cookiemonster.ParseFile(*cookiesPath)
	if err != nil {
		logFatal("echo360: failed to parse cookies file: %v", err)
	}

	u.Path = path.Join("/section", parts[2], "syllabus")

	workList, err := parseSyllabus(u, cookies)
	if err != nil {
		logFatal("echo360: failed to parse syllabus: %v", err)
	}

	uiprogress.Start()
	defer uiprogress.Stop()

	var wg sync.WaitGroup
	workCh := make(chan *work, len(workList))
	defer close(workCh)

	for i := range workList {
		wg.Add(1)
		workCh <- &workList[i]
	}

	stop := make(chan struct{})

	for i := 0; i < *threads && i < len(workList); i++ {
		go downloader(*dir, cookies, &wg, workCh, stop)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		wg.Wait()
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)

	select {
	case <-done:
	case <-sig:
		signal.Stop(sig)

		close(stop)
		wg.Add(-len(workCh))
		<-done
	}
}

func downloader(dir string, cookies []*http.Cookie, wg *sync.WaitGroup, workCh <-chan *work, stop <-chan struct{}) {
	buf := make([]byte, 64<<10)

	for {
		select {
		case work, ok := <-workCh:
			if !ok {
				break
			}

			if err := work.download(buf, dir, cookies); err != nil {
				logError("echo360: failed to download %q: %v", work.name, err)
			}

			wg.Done()
		case <-stop:
			break
		}
	}
}
