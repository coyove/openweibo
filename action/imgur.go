package action

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coyove/iis/common"
)

var imgurThrot = struct {
	sync.Mutex
	counter int
	start   time.Time
}{
	counter: 0,
	start:   time.Now(),
}

func uploadImgur(image string) (string, error) {
	imgurThrot.Lock()
	// 50 posts per hour, we use 45, so that is 0.0125 per sec
	diff := time.Since(imgurThrot.start).Seconds()
	if diff > 3600 {
		imgurThrot.start = time.Unix(time.Now().Unix()-1, 0)
		imgurThrot.counter = 0
	}

	if float64(imgurThrot.counter)/time.Since(imgurThrot.start).Seconds() > 0.01 {
		imgurThrot.Unlock()

		sec := time.Unix(imgurThrot.start.Unix()+int64(float64(imgurThrot.counter)/0.01), 0).Sub(time.Now()).Seconds()
		return "", fmt.Errorf("image/throt/%.1fs", sec)
	}
	imgurThrot.counter++
	imgurThrot.Unlock()

	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)
	writer.WriteField("image", image[strings.Index(image, ",")+1:])
	if err := writer.Close(); err != nil {
		return "", err
	}

	req, _ := http.NewRequest("POST", "https://api.imgur.com/3/image", payload)
	req.Header.Add("Authorization", "Client-ID 6204e68f30045a1")
	req.Header.Set("Content-Type", writer.FormDataContentType())

	c := &http.Client{Timeout: 2 * time.Second}

	if common.Cfg.Key == "0123456789abcdef" {
		// debug
		c.Transport = &http.Transport{
			Proxy: func(*http.Request) (*url.URL, error) {
				return url.Parse("socks5://127.0.0.1:1080")
			},
		}
		c.Timeout = 0
	}

	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}

	buf, _ := ioutil.ReadAll(resp.Body)
	resp.Body.Close()

	var p struct {
		Data struct {
			Link string `json:"link"`
		} `json:"data"`
		Success bool `json:"success"`
	}

	if err := json.Unmarshal(buf, &p); err != nil || !p.Success {
		if len(buf) > 1024 {
			buf = buf[:1024]
		}
		return "", fmt.Errorf("resp error: %q", buf)
	}

	return p.Data.Link, nil
}
