package common

import (
	"crypto/tls"
	"fmt"
	"html/template"
	"net/smtp"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	rxSan        = regexp.MustCompile(`(?m)(\n|\[imgs\][\s\S]+[/imgs\][\s\n]*|\[code\][\s\S]+?\[/code\]|<|https?://[^\s<>"'#\[\]]+|@\S+|#[^# ]+)|\[mj\]\d+\[/mj\]`)
	rxFirstImage = regexp.MustCompile(`(?i)(https?://\S+\.(png|jpg|gif|webp|jpeg)|\[img\]https?://\S+\[/img\])`)
	rxMentions   = regexp.MustCompile(`((@|#)[^@# ]+)`)
	rxAcCode     = regexp.MustCompile(`v\/ac(\d+)`)
	rxBiliAVCode = regexp.MustCompile(`(av(\d+)|BV(\w+))`)
	rxWYYYCode   = regexp.MustCompile(`([^r]id=|song/)(\d+)`)
	rxYTCode     = regexp.MustCompile(`(youtu\.be\/(\w+)|v=(\w+))`)

	RevRenderTemplateString func(name string, v interface{}) string
)

func SoftTrunc(a string, n int) string {
	a = strings.TrimSpace(a)
	if len(a) <= n+2 {
		return a
	}
	a = a[:n+2]
	for len(a) > 0 && a[len(a)-1]>>6 == 2 {
		a = a[:len(a)-1]
	}
	if len(a) == 0 {
		return a
	}
	a = a[:len(a)-1]
	return a + "..."
}

func SoftTruncDisplayWidth(a string, w int) string {
	r := []rune(strings.TrimSpace(a))
	width := 0
	for i := range r {

		if r[i] > 0x2000 {
			width += 2
		} else {
			width++
		}

		if width >= w {
			r = r[:i]
			break
		}
	}
	return string(r)
}

func DetectMedia(media string) string {
	media = SoftTrunc(media, 1024)
	if media == "" {
		return ""
	}
	if strings.Count(media, ";") < 16 {
		return "IMG:" + media
	}
	return "IMG:" + strings.Join(strings.Split(media, ";")[:16], ";")
}

func SanText(in string) string {
	newLines := 0
	in = rxSan.ReplaceAllStringFunc(in, func(in string) string {
		if in == "\n" {
			if newLines++; newLines < 10 {
				// We allow 10 new lines in an article at max
				return in
			}
			return " "
		}
		if in == "<" {
			return "&lt;"
		}
		if strings.HasPrefix(in, "[code]") && strings.HasSuffix(in, "[/code]") {
			return "<code>" + strings.TrimSpace(in[6:len(in)-7]) + "</code>"
		}
		// if strings.HasPrefix(in, "[imgs]") {
		// 	in = strings.TrimPrefix(strings.TrimSpace(in), "[imgs]")
		// 	in = RevRenderTemplateString("image.html", strings.Split(strings.TrimSuffix(in, "[/imgs]"), "\n"))
		// 	return strings.TrimSpace(in)
		// }
		if strings.HasPrefix(in, "[mj]") {
			return "<img class=majiang src='https://static.saraba1st.com/image/smiley/face2017/" + in[4:len(in)-5] + ".png'>"
		}
		if len(in) > 0 {
			s := SafeStringForCompressString(template.HTMLEscapeString(in[1:]))
			if in[0] == '#' {
				// AddTagToSearch(in[1:])
				return "<a href='/tag/" + s + "'>" + in + "</a>"
			}
			if in[0] == '@' {
				// AddUserToSearch(in[1:])
				return "<a href='javascript:void(0)'" +
					` class=mentioned-user` +
					` onclick="showInfoBox(this,'` + in[1:] + `')">` +
					in + "</a>"
			}
		}
		if strings.Contains(in, "bilibili") || strings.Contains(in, "b23.tv") {
			res := rxBiliAVCode.FindAllStringSubmatch(in, 1)
			if len(res) == 1 && len(res[0]) > 2 {
				if strings.HasPrefix(res[0][0], "BV") { // new BV code
					return makeVideoButton("#00a1d6",
						res[0][0],
						"https://player.bilibili.com/player.html?bvid="+res[0][0],
						"https://www.bilibili.com/"+res[0][0])
				}
				return makeVideoButton("#00a1d6",
					"av"+res[0][2],
					"https://player.bilibili.com/player.html?aid="+res[0][2],
					"https://www.bilibili.com/av"+res[0][2])
			}
		}

		if strings.Contains(in, "acfun") {
			res := rxAcCode.FindAllStringSubmatch(in, 1)
			if len(res) == 1 && len(res[0]) == 2 {
				return makeVideoButton("#fd4c5d",
					"ac"+res[0][1],
					"https://www.acfun.cn/player/ac"+res[0][1],
					"https://www.acfun.cn/v/ac"+res[0][1])
			}
		}

		if strings.Contains(in, "music.163") {
			res := rxWYYYCode.FindAllStringSubmatch(in, 1)
			if len(res) == 1 && len(res[0]) == 3 {
				return strings.Replace(makeVideoButton("#e60125",
					"yy"+res[0][2],
					"https://music.163.com/outchain/player?type=2&auto=0&height=66&id="+res[0][2],
					"https://music.163.com/song?id="+res[0][2]), "<iframe ", "<iframe fixed-height=80 ", 1)
			}
		}

		if strings.HasPrefix(in, "https://youtu") {
			res := rxYTCode.FindAllStringSubmatch(in, 1)
			if len(res) == 1 && len(res[0]) >= 3 {
				return makeVideoButton("#db4437",
					"yt"+res[0][2],
					"https://www.youtube.com/embed/"+res[0][2],
					in)
			}
		}

		return "<a href='" + in + "' target=_blank>" + in + "</a>"
	})
	return in
}

func makeVideoButton(color string, title, vid, url string) string {
	return "<a style='color:" + color + "' href='" + url + "' target=_blank><i class='icon-export-alt'></i></a>" +
		"<a style='color:" + color + "' href='javascript:void(0)' " +
		"onclick='adjustVideoIFrame(this,\"" + vid + "\")'>" + title + "</a>" +
		"<iframe style='width:100%;display:none' frameborder=0 allowfullscreen=true></iframe>"
}

func ExtractMentionsAndTags(in string) ([]string, []string) {
	res := rxMentions.FindAllString(in, Cfg.MaxMentions)
	mentions, tags := []string{}, []string{}
	for i := range res {
		res[i] = res[i][:1] + SafeStringForCompressString(res[i][1:])
	}

AGAIN: // TODO
	for i := range res {
		for j := range res {
			if i != j && res[i] == res[j] {
				res = append(res[:j], res[j+1:]...)
				goto AGAIN
			}
		}
		if res[i][0] == '#' {
			tags = append(tags, strings.TrimRight(res[i][1:], "#"))
		} else {
			mentions = append(mentions, res[i][1:])
		}
	}
	return mentions, tags
}

func ExtractFirstImage(in string) string {
	m := rxFirstImage.FindAllString(in, 1)
	if len(m) > 0 {
		if strings.HasPrefix(m[0], "[img]") && strings.HasSuffix(m[0], "[/img]") {
			return m[0][5 : len(m[0])-6]
		}
		return m[0]
	}
	return ""
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

type loginAuth struct {
	username, password string
}

func LoginAuth(username, password string) smtp.Auth {
	return &loginAuth{username, password}
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", []byte{}, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		switch string(fromServer) {
		case "Username:":
			return []byte(a.username), nil
		case "Password:":
			return []byte(a.password), nil
		default:
			return nil, fmt.Errorf("unknown command: %q", fromServer)
		}
	}
	return nil, nil
}

func SendMail(to, subject, body string) error {
	server, me, password := Cfg.SMTPServer, Cfg.SMTPEmail, Cfg.SMTPPassword
	c, err := smtp.Dial(server)
	if err != nil {
		return err
	}

	if err := c.StartTLS(&tls.Config{InsecureSkipVerify: true}); err != nil {
		return err
	}

	if err := c.Auth(&loginAuth{me, password}); err != nil {
		return err
	}

	if err := c.Mail(me); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}

	wc, err := c.Data()
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(wc, ("To: " + to + "\r\n" + "Subject: " + subject + "\r\n\r\n" + body + "\r\n")); err != nil {
		return err
	}

	if err := wc.Close(); err != nil {
		return err
	}
	return c.Quit()
}

func Hash32(n string) (h uint32) {
	h = 2166136261
	for i := 0; i < len(n); i++ {
		h = h * 16777619
		h = h ^ uint32(n[i])
	}
	return
}

func Hash16(n string) (h uint16) {
	return uint16(Hash32(n))
}

func ParseDuration(v string) time.Duration {
	if strings.HasSuffix(v, "d") {
		d, _ := strconv.Atoi(v[:len(v)-1])
		return time.Duration(d) * time.Hour * 24
	}
	d, _ := time.ParseDuration(v)
	return d
}
