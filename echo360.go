package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"runtime"
	"strings"
	"sync"

	"github.com/MercuryEngineering/CookieMonster"
	"github.com/c2h5oh/datasize"
	"github.com/gosuri/uiprogress"
	"github.com/tmthrgd/httputils"
)

const userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/51.0.2704.103 Safari/537.36"

func init() {
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "Usage of %s:\n", os.Args[0])
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
			logFatal("echo360: %v", err)
		}
	}

	u, err := url.Parse(flag.Arg(0))
	if err != nil {
		logFatal("echo360: %v", err)
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
		logFatal("echo360: %v", err)
	}

	u.Path = path.Join("/section", parts[2], "syllabus")

	resp, err := httpGet(u.String(), cookies)
	if err != nil {
		logFatal("echo360: %v", err)
	}

	defer resp.Body.Close()

	if !httputils.MIMETypeMatches(resp.Header.Get("Content-Type"), []string{"application/json"}) {
		logFatal("echo360: unsupported mime type (possibly invalid credentials)")
	}

	var data struct {
		Status  string
		Message string
		Data    []struct {
			Type   string
			Lesson struct {
				Lesson struct {
					DisplayName string
				}
				Video struct {
					Media struct {
						Name  string
						Media struct {
							Type    string
							Current struct {
								PrimaryFiles []struct {
									S3URL string
									Width int
								}
							}
						}
					}
				}
				Medias []struct {
					DownloadURI    string
					IsAvailable    bool
					IsDownloadable bool
				}
			}
		}
	}

	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&data); err != nil {
		logFatal("echo360: %v", err)
	}

	if dec.More() {
		logNotice("echo360: trailing JSON garbage")
	}

	if data.Status != "ok" {
		logFatal("echo360: server returned JSON error %q: %q", data.Status, data.Message)
	}

	var workList []work

outer:
	for _, lesson := range data.Data {
		if lesson.Type != "SyllabusLessonType" {
			logInfo("echo360: unknown lesson type %q", lesson.Type)
			continue
		}

		if media := lesson.Lesson.Video.Media; media.Media.Type == "VideoPresentation" {
			var (
				width int
				s3URL string
			)
			for _, file := range media.Media.Current.PrimaryFiles {
				if file.Width < width {
					continue
				}

				width, s3URL = file.Width, file.S3URL
			}

			if s3URL != "" {
				workList = append(workList, work{
					media.Name,
					s3URL,
				})

				continue outer
			}
		}

		for _, media := range lesson.Lesson.Medias {
			if !media.IsAvailable || !media.IsDownloadable {
				continue
			}

			r, err := url.Parse(media.DownloadURI)
			if err != nil {
				logFatal("echo360: %v", err)
			}

			workList = append(workList, work{
				lesson.Lesson.Lesson.DisplayName,
				u.ResolveReference(r).String(),
			})

			continue outer
		}

		logInfo("echo360: could not find downloadable video for lesson %q", lesson.Lesson.Lesson.DisplayName)
	}

	logInfo("echo360: found %d videos for %d lessons", len(workList), len(data.Data))

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

type work struct {
	name string
	url  string
}

func downloader(dir string, cookies []*http.Cookie, wg *sync.WaitGroup, workCh <-chan *work, stop <-chan struct{}) {
	buf := make([]byte, 64<<10)

	for {
		var work *work
		select {
		case <-stop:
			return
		case work = <-workCh:
		}

		// BUG(tmthrgd): investigate why a panic may occur on first line of download
		if work == nil {
			var stopped bool
			select {
			case <-stop:
				stopped = true
			default:
			}

			logFatal("work is <nil> with stoped = %v", stopped)
		}

		if err := download(buf, dir, cookies, work); err != nil {
			logError("echo360: failed to download %q: %v", work.name, err)
		}

		wg.Done()
	}
}

func download(buf []byte, dir string, cookies []*http.Cookie, work *work) error {
	u, err := url.Parse(work.url)

	var ext string
	if err == nil {
		ext = path.Ext(u.Path)
	}
	if ext == "" {
		ext = ".mp4"
	}

	name := work.name + ext
	if _, err := os.Stat(path.Join(dir, name)); err == nil {
		return nil
	}

	resp, err := httpGet(work.url, cookies)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	// video/mp4

	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		_, params, err := mime.ParseMediaType(cd)
		if err != nil {
			return err
		}

		if filename, ok := params["filename"]; ok {
			name = filename
		}
	}

	if _, err := os.Stat(path.Join(dir, name)); err == nil {
		return nil
	}

	f, err := ioutil.TempFile(dir, ".echo360-")
	if err != nil {
		return err
	}

	defer os.Remove(f.Name())
	defer f.Close()

	var (
		body io.Reader = resp.Body
		bar  *uiprogress.Bar
	)
	if resp.ContentLength > 0 {
		bar = uiprogress.AddBar(int(resp.ContentLength)).AppendCompleted().AppendFunc(bytesComplete)
		body = &progressReader{body, bar}
	} else {
		bar = uiprogress.AddBar(1).AppendCompleted()
		defer bar.Set(1)
	}

	if _, err := io.CopyBuffer(f, body, buf); err != nil {
		return err
	}

	return os.Rename(f.Name(), path.Join(dir, name))
}

func bytesComplete(b *uiprogress.Bar) string {
	cur := datasize.ByteSize(b.Current())
	tot := datasize.ByteSize(b.Total)
	return fmt.Sprintf("%s of %s", cur.HR(), tot.HR())
}

type progressReader struct {
	r   io.Reader
	bar *uiprogress.Bar
}

func (pr *progressReader) Read(p []byte) (n int, err error) {
	n, err = pr.r.Read(p)
	pr.bar.Set(pr.bar.Current() + n)
	return
}
