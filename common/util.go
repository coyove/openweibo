package common

import (
	"html/template"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/coyove/iis/common/compress"
	"github.com/gin-gonic/gin"
)

var (
	rxSan        = regexp.MustCompile(`(?m)(\n|\[hide\][\s\S]+?\[/hide\][\s\n]*|\[code\][\s\S]+?\[/code\]|<|https?://[^\s<>"'#\[\]]+|@\S+|#[^# \s\n\t]+)|\[mj\](\d+|ac\d+|a2_\d+)\[/mj\]`)
	rxMentions   = regexp.MustCompile(`((@|#)[^@# \n\s\t]+)`)
	rxAcCode     = regexp.MustCompile(`v\/ac(\d+)`)
	rxBiliAVCode = regexp.MustCompile(`(av(\d+)|BV(\w+))`)
	rxWYYYCode   = regexp.MustCompile(`([^r]id=|song/)(\d+)`)
	rxYTCode     = regexp.MustCompile(`(youtu\.be\/(\w+)|v=(\w+))`)
	rxCrawler    = regexp.MustCompile(`(?i)(bot|googlebot|crawler|spider|robot|crawling)`)
	keyLocks     [65536]sync.Mutex

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
		if strings.HasPrefix(in, "[hide]") {
			in = strings.TrimPrefix(strings.TrimSpace(in), "[hide]")
			in = strings.TrimSuffix(in, "[/hide]")
			return "<span class=hidden-text>" + strings.TrimSpace(in) + "</span>"
		}
		if strings.HasPrefix(in, "[mj]") {
			var idx = in[4 : len(in)-5]
			var host string
			if strings.HasPrefix(idx, "a") {
				host = "<img class='majiang ac-emoji' src='/s/emoji/"
			} else {
				host = "<img class='majiang' src='https://static.saraba1st.com/image/smiley/face2017/"
			}
			return host + idx + ".png'>"
		}
		if len(in) > 0 {
			s := compress.SafeStringForCompressString(template.HTMLEscapeString(in[1:]))
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
		res[i] = res[i][:1] + compress.SafeStringForCompressString(res[i][1:])
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

func Hash32(n string) (h uint32) {
	h = 2166136261
	for i := 0; i < len(n); i++ {
		h = h * 16777619
		h = h ^ uint32(n[i])
	}
	return
}

func Hash16(n string) (h uint16) { return uint16(Hash32(n)) }

func ParseDuration(v string) time.Duration {
	if strings.HasSuffix(v, "d") {
		d, _ := strconv.Atoi(v[:len(v)-1])
		return time.Duration(d) * time.Hour * 24
	}
	d, _ := time.ParseDuration(v)
	return d
}

func PushIP(dataIP, ip string) string {
	if ips := append(strings.Split(dataIP, ","), ip); len(ips) > 5 {
		return strings.Join(ips[len(ips)-5:], ",")
	} else {
		return strings.Join(ips, ",")
	}
}

func Err2(v interface{}, e error) error { return e }

func BoolInt(v bool) int { return int(*(*byte)(unsafe.Pointer(&v))) }

func BoolInt2(v bool) int { return int((float64(*(*byte)(unsafe.Pointer(&v))) - 0.5) * 2) }

func DefaultMap(m map[string]string) map[string]string {
	if m == nil {
		m = map[string]string{}
	}
	return m
}

func LockKey(key string) {
	keyLocks[Hash16(key)].Lock()
}

func LockAnotherKey(key, basedOnKey string) bool {
	if Hash16(basedOnKey) == Hash16(key) {
		return false
	}
	keyLocks[Hash16(key)].Lock()
	return true
}

func UnlockKey(key string) { keyLocks[Hash16(key)].Unlock() }

func IsCrawler(g *gin.Context) bool {
	if rxCrawler.MatchString(g.Request.UserAgent()) {
		return true
	}
	v, _ := g.Cookie("crawler")
	return v == "1"
}
