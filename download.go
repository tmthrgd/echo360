package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"

	"github.com/c2h5oh/datasize"
	"github.com/gosuri/uiprogress"
)

const barNameLength = 15

type work struct {
	name string
	url  string
}

func (w *work) download(buf []byte, dir string, cookies []*http.Cookie) error {
	u, err := url.Parse(w.url)
	if err != nil {
		return err
	}

	ext := path.Ext(u.Path)
	if ext == "" {
		ext = ".mp4"
	}

	name := w.name + ext
	if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
		return nil
	}

	resp, err := httpGet(w.url, cookies)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		_, params, err := mime.ParseMediaType(cd)
		if err != nil {
			return err
		}

		if filename, ok := params["filename"]; ok {
			name = filename
		}

		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return nil
		}
	}

	f, err := ioutil.TempFile(dir, ".echo360-")
	if err != nil {
		return err
	}

	closer := ioOnceCloser(f)

	defer os.Remove(f.Name())
	defer closer.Close()

	var (
		body io.Reader = resp.Body
		bar  *uiprogress.Bar
	)
	if resp.ContentLength > 0 {
		bar = progress.AddBar(int(resp.ContentLength)).
			AppendCompleted().AppendFunc(bytesComplete)
		body = &progressReader{body, bar}
	} else {
		bar = progress.AddBar(1).AppendCompleted()
		defer bar.Set(1)
	}

	barName := strutilResize(w.name, barNameLength)
	bar.PrependFunc(func(*uiprogress.Bar) string {
		return barName
	})

	if _, err := io.CopyBuffer(f, body, buf); err != nil {
		return err
	}

	if err := closer.Close(); err != nil {
		return err
	}

	return os.Rename(f.Name(), filepath.Join(dir, name))
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
