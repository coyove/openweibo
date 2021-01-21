package common

import (
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/coyove/iis/common/compress"
	"github.com/gin-gonic/gin"
)

const atTagRx = `((@|#)[^@# \n\s\t]+)`

var (
	rxSan      = regexp.MustCompile(`(?m)(<|\n|\[\S+?\][\s\S]+?\[/\S+?\]|https?://[^\s<>"'#\[\]]+|` + atTagRx + `)`)
	rxMentions = regexp.MustCompile(atTagRx)
	rxCrawler  = regexp.MustCompile(`(?i)(bot|googlebot|crawler|spider|robot|crawling)`)
	keyLocks   [65536]sync.Mutex

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
			newLines++ // 10 lines at max
			return IfStr(newLines < 10, in, " ")
		}
		if in == "<" {
			return "&lt;"
		}
		if x := truncCodeTag(in, "code"); x != "" {
			return "<code>" + x + "</code>"
		}
		if x := truncCodeTag(in, "append"); x != "" {
			return "<span class=append-by>" + x + "</span>"
		}
		if x := truncCodeTag(in, "hide"); x != "" {
			return "<span class=hidden-text>" + x + "</span>"
		}
		if x := truncCodeTag(in, "mj"); x != "" {
			if strings.HasPrefix(x, "ac") {
				return "<img class='majiang ac-emoji' src='/s/emoji/" + x + ".png'>"
			}
			return "<img class='majiang' src='https://static.saraba1st.com/image/smiley/face2017/" + x + ".png'>"
		}
		if len(in) > 0 {
			s := compress.SafeStringForCompressString(in[1:])
			if in[0] == '#' {
				return "<a target=_blank href='/tag/" + s + "'>" + in + "</a>"
			}
			if in[0] == '@' {
				return `<a href='javascript:void(0)' class=mentioned-user onclick="showInfoBox(this,'` + s + `')">` + in + "</a>"
			}
		}

		return "<a href='" + in + "' target=_blank>" + in + "</a>"
	})
	return in
}

func truncCodeTag(in string, tag string) string {
	p, s := strings.HasPrefix, strings.HasSuffix
	if p(in, "[") && p(in[1:], tag) && p(in[1+len(tag):], "]") {
		if s(in, "]") && s(in[:len(in)-1], tag) && s(in[:len(in)-1-len(tag)], "[/") {
			return in[1+len(tag)+1 : len(in)-1-len(tag)-2]
		}
	}
	return ""
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

func IfStr(v bool, t, f string) string { return [2]string{f, t}[BoolInt(v)] }

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

func UnlockKey(key string) {
	keyLocks[Hash16(key)].Unlock()
}

func IsCrawler(g *gin.Context) bool {
	if rxCrawler.MatchString(g.Request.UserAgent()) {
		return true
	}
	v, _ := g.Cookie("crawler")
	return v == "1"
}
