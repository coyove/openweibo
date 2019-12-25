package mv

import (
	"html/template"
	"regexp"
	"strings"
	"time"

	"github.com/coyove/iis/cmd/ch/config"
)

var (
	rxSan        = regexp.MustCompile(`(?m)(\[img\]https?://\S+\[/img\]|\[code\][\s\S]+\[/code\]|<|https?://[^\s<>"'#\[\]]+|@\S+|#\S+)`)
	rxFirstImage = regexp.MustCompile(`(?i)(https?://\S+\.(png|jpg|gif|webp|jpeg)|\[img\]https?://\S+\[/img\])`)
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
		if strings.HasPrefix(in, "[code]") && strings.HasSuffix(in, "[/code]") {
			return "<code>" + strings.TrimSpace(in[6:len(in)-7]) + "</code>"
		}
		if strings.HasPrefix(in, "[img]") && strings.HasSuffix(in, "[/img]") {
			return ""
		}
		if len(in) > 0 {
			s := SafeStringForCompressString(template.HTMLEscapeString(in[1:]))
			if in[0] == '#' {
				AddTagToSearch(in[1:])
				return "<a href='/tag/" + s + "' target=_blank>" + in + "</a>"
			}
			if in[0] == '@' {
				AddUserToSearch(in[1:])
				return "<a href='/t/" + s + "' target=_blank>" + in + "</a>"
			}
		}
		return "<a href='" + in + "' target=_blank>" + in + "</a>"
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
		if strings.HasPrefix(m[0], "[img]") && strings.HasSuffix(m[0], "[/img]") {
			return m[0][5 : len(m[0])-6]
		}
		return m[0]
	}
	return ""
}
