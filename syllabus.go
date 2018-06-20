package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/tmthrgd/httputils"
)

func parseSyllabus(u *url.URL, cookies []*http.Cookie) ([]*work, error) {
	resp, err := httpGet(u.String(), cookies)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); !httputils.MIMETypeMatches(ct, []string{"application/json"}) {
		return nil, fmt.Errorf("echo360: unsupported media type %q (possibly invalid credentials)", ct)
	}

	var data jsonSchema

	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&data); err != nil {
		return nil, err
	}

	if dec.More() {
		logNotice("echo360: trailing JSON garbage")
	}

	if data.Status != "ok" {
		return nil, fmt.Errorf("echo360: server returned JSON error %q: %q", data.Status, data.Message)
	}

	workList := make([]*work, 0, len(data.Data))

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
				workList = append(workList, &work{
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
				return nil, err
			}

			workList = append(workList, &work{
				lesson.Lesson.Lesson.DisplayName,
				u.ResolveReference(r).String(),
			})

			continue outer
		}

		logInfo("echo360: could not find downloadable video for lesson %q", lesson.Lesson.Lesson.DisplayName)
	}

	logInfo("echo360: found %d videos for %d lessons", len(workList), len(data.Data))
	return workList, nil
}

type jsonSchema struct {
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
