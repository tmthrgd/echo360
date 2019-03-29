package main

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"

	cookiemonster "github.com/MercuryEngineering/CookieMonster"
	"golang.org/x/net/publicsuffix"
)

const userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/51.0.2704.103 Safari/537.36"

func httpClient(baseURL *url.URL, cookiesPath string) (*http.Client, error) {
	cookies, err := cookiemonster.ParseFile(cookiesPath)
	if err != nil {
		return nil, fmt.Errorf("echo360: failed to parse cookie file: %v", err)
	}

	unixZero := time.Unix(0, 0)
	for _, cookie := range cookies {
		// cookiejar uses IsZero to determine whether a cookie has
		// expired which doesn't accept unixZero  which cookiejar uses
		// for "forever" cookies. We have to replace those values with
		// the actual zero value of time.Time to satisfy cookiejar.
		if cookie.Expires.Equal(unixZero) {
			cookie.Expires = time.Time{}
		}
	}

	jar, _ := cookiejar.New(&cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
	})
	jar.SetCookies(baseURL, cookies)

	return &http.Client{
		Jar: jar,
	}, nil
}

func httpGet(url string, client *http.Client) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("echo360: server returned HTTP %d error: %q", resp.StatusCode, resp.Status)
	}

	return resp, nil
}
