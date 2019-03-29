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

	"github.com/gosuri/uiprogress"
)

var (
	progress        = uiprogress.New()
	progressStarted bool
)

func init() {
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "Usage of %s:\n", filepath.Base(os.Args[0]))
		fmt.Fprintf(out, "%s [flags] <url> \n", os.Args[0])
		flag.PrintDefaults()
	}

	progress.Width -= barNameLength
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

	u.Path = path.Join("/section", parts[2], "syllabus")

	if *cookiesPath == "" {
		logFatal("echo360: -cookies flag cannot be empty")
	}

	client, err := httpClient(u, *cookiesPath)
	if err != nil {
		logFatal("echo360: failed to create http client: %v", err)
	}

	workList, err := parseSyllabus(u, client)
	if err != nil {
		logFatal("echo360: failed to parse syllabus: %v", err)
	}

	if err := os.MkdirAll(*dir, 0755); err != nil {
		logFatal("echo360: failed to create directory: %v", err)
	}

	logMu.Lock()
	progressStarted = true
	progress.Start()
	defer progress.Stop()
	logMu.Unlock()

	workCh := make(chan *work, len(workList))
	for _, work := range workList {
		workCh <- work
	}

	close(workCh)

	var wg sync.WaitGroup
	wg.Add(len(workList))

	stop := make(chan struct{})

	for i := 0; i < *threads && i < len(workList); i++ {
		go downloader(*dir, client, &wg, workCh, stop)
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

		logNotice("echo360: ^C received, finishing ongoing downloads; ^C again to terminate")

		for range workCh {
			wg.Done()
		}

		<-done
	}
}

func downloader(dir string, client *http.Client, wg *sync.WaitGroup, workCh <-chan *work, stop <-chan struct{}) {
	buf := make([]byte, 64<<10)

loop:
	for {
		select {
		case work, ok := <-workCh:
			if !ok {
				break loop
			}

			// stop always has priority, so check it again
			// as select statements are randomly ordered.
			select {
			case <-stop:
				wg.Done()
				break loop
			default:
			}

			if err := work.download(buf, dir, client); err != nil {
				logError("echo360: failed to download %q: %v", work.name, err)
			}

			wg.Done()
		case <-stop:
			break loop
		}
	}
}
