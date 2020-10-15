package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type files []string

func (i *files) String() string { return fmt.Sprint(*i) }

func (i *files) Set(value string) error { *i = append(*i, value); return nil }

var (
	medias   files
	host     = flag.String("h", "http://127.0.0.1:5010", "")
	content  = flag.String("c", "hello bot", "")
	token    = flag.String("t", "bad_token", "")
	noMaster = flag.Bool("no-master", false, "")
	client   = &http.Client{}
)

func upload(rd *os.File) (string, error) {
	defer rd.Close()
	url := *host + "/api/upload_image?api2_uid=" + *token

	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)

	part1, err := writer.CreateFormFile("i", rd.Name())
	if err != nil {
		return "", err
	}

	if _, err := io.Copy(part1, rd); err != nil {
		return "", err
	}

	if err := writer.Close(); err != nil {
		return "", err
	}

	req, _ := http.NewRequest("POST", url, payload)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}

	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)

	switch r := string(body); {
	case strings.HasPrefix(r, "LOCAL"):
		return r, nil
	case strings.HasPrefix(r, "CHECK("):
		idx := strings.Index(r, ")-")
		url, tag := r[6:idx], r[idx+2:]
		for {
			time.Sleep(time.Second)
			err := func() error {
				resp, err := http.Get(url)
				if err != nil {
					return err
				}
				if resp.StatusCode == 200 {
					return nil
				}
				return errors.New(resp.Status)
			}()
			if err == nil {
				log.Println("OK", url)
				break
			} else {
				log.Println("CHECK", url, err)
			}
		}
		return tag, nil
	default:
		return "", errors.New(r)
	}
}

func post(replyTo string, content string, media []string, ex func(v url.Values)) (string, error) {
	form := url.Values{}
	form.Add("api2_uid", *token)
	form.Add("content", content)
	if replyTo != "" {
		form.Add("parent", replyTo)
	}
	if ex != nil {
		ex(form)
	}

	if len(media) > 0 {
		tags := []string{}
		for _, m := range media {
			f, err := os.Open(m)
			if err != nil {
				return "", err
			}
			tag, err := upload(f)
			if err != nil {
				return "", err
			}
			tags = append(tags, tag)
		}
		form.Add("media", strings.Join(tags, ";"))
	}

	req, _ := http.NewRequest("POST", *host+"/api2/new", strings.NewReader(form.Encode()))

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	res, err := client.Do(req)
	if err != nil {
		return "", err
	}

	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)

	if bytes.HasPrefix(body, []byte("ok:")) {
		return res.Header.Get("X-Article-Code"), nil
	}
	return "", errors.New(string(body))
}

func main() {
	flag.Var(&medias, "f", "")
	flag.Parse()

	*host = strings.TrimRight(*host, "/")
	log.Println(post("", *content, medias, func(kv url.Values) {
		if *noMaster {
			kv.Add("no_master", "1")
		}
	}))
}
