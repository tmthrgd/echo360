package main

import (
	"fmt"
	"net/http"
)

func httpGet(url string, cookies []*http.Cookie) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}

	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("echo360: server returned HTTP %d error: %q", resp.StatusCode, resp.Status)
	}

	return resp, nil
}
