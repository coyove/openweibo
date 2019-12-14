package action

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/ident"
	"github.com/coyove/iis/cmd/ch/manager"
	"github.com/coyove/iis/cmd/ch/mv"
	"github.com/gin-gonic/gin"
)

var m *manager.Manager

func SetManager(mgr *manager.Manager) {
	m = mgr
}

func EncodeQuery(a ...string) string {
	query := url.Values{}
	for i := 0; i < len(a); i += 2 {
		if a[i] != "" {
			query.Add(a[i], a[i+1])
		}
	}
	return "?" + query.Encode()
}

func checkToken(g *gin.Context) string {
	var (
		uuid       = mv.SoftTrunc(g.PostForm("uuid"), 32)
		_, tokenok = ident.ParseToken(g, uuid)
	)

	if !g.GetBool("ip-ok") {
		return fmt.Sprintf("guard/cooling-down/%.1fs", float64(config.Cfg.Cooldown)-g.GetFloat64("ip-ok-remain"))
	}

	if u, ok := g.Get("user"); ok {
		if u.(*mv.User).Banned {
			return "guard/id-not-existed"
		}
	}

	// Admin still needs token verification
	if !tokenok {
		return "guard/token-expired"
	}

	return ""
}

func throtUser(g *gin.Context) string {
	u2, _ := g.Get("user")
	u, _ := u2.(*mv.User)

	if u == nil || u.Banned {
		return "guard/id-not-existed"
	}

	return ""
}

func sanUsername(id string) string {
	return ident.SafeStringForCompressString(id)
}

func checkCaptcha(g *gin.Context) string {
	var (
		answer            = mv.SoftTrunc(g.PostForm("answer"), 6)
		uuid              = mv.SoftTrunc(g.PostForm("uuid"), 32)
		tokenbuf, tokenok = ident.ParseToken(g, uuid)
		challengePassed   bool
	)

	if !g.GetBool("ip-ok") {
		return fmt.Sprintf("guard/cooling-down/%.1fs", float64(config.Cfg.Cooldown)-g.GetFloat64("ip-ok-remain"))
	}

	if !tokenok {
		return "guard/token-expired"
	}

	if len(answer) == 4 {
		challengePassed = true
		for i := range answer {
			a := answer[i]
			if a >= 'A' && a <= 'Z' {
				a = a - 'A' + 'a'
			}

			if a != "0123456789acefhijklmnpqrtuvwxyz"[tokenbuf[i]%31] &&
				a != "oiz3asg7b9acefhijklmnpqrtuvwxyz"[tokenbuf[i]%31] {
				challengePassed = false
				break
			}
		}
	}

	if !challengePassed {
		log.Println(g.MustGet("ip"), "challenge failed")
		return "guard/failed-captcha"
	}

	if !tokenok {
		return "guard/token-expired"
	}

	return ""
}

func genSession() string {
	p := [12]byte{}
	rand.Read(p[:])
	for i := range p {
		if p[i] == 0 {
			p[i] = 1
		}
	}
	return base64.StdEncoding.EncodeToString(p[:])
}

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

	if config.Cfg.Key == "0123456789abcdef" {
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
