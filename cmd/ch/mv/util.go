package mv

import (
	"html/template"
	"regexp"
	"strings"
	"time"

	"github.com/coyove/iis/cmd/ch/config"
)

var (
	rxSan        = regexp.MustCompile(`(?m)(^>.+|<|https?://\S+)`)
	rxFirstImage = regexp.MustCompile(`(?i)https?://\S+\.(png|jpg|gif|webp|jpeg)`)
	rxMentions   = regexp.MustCompile(`((@|#)\S+)`)
)

func FormatTime(x time.Time, rich bool) string {
	return x.UTC().Add(8 * time.Hour).Format("2006-01-02 15:04:05")
}

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

func sanText(in string) string {
	in = rxSan.ReplaceAllStringFunc(in, func(in string) string {
		if in == "<" {
			return "&lt;"
		}
		if strings.HasPrefix(in, ">") {
			return "<code>" + strings.TrimSpace(in[1:]) + "</code>"
		}
		return "<a href='" + in + "' target=_blank>" + in + "</a>"
	})
	in = rxMentions.ReplaceAllStringFunc(in, func(in string) string {
		if len(in) < 2 {
			return in
		}
		s := SafeStringForCompressString(template.HTMLEscapeString(in[1:]))
		if in[0] == '#' {
			return "<a href='/tag/" + s + "'>" + in + "</a>"
		}
		return "<a href='/t/" + s + "'>" + in + "</a>"
	})
	return in
}

func ExtractMentionsAndTags(in string) ([]string, []string) {
	res := rxMentions.FindAllString(in, config.Cfg.MaxMentions)
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
		return m[0]
	}
	return ""
}
