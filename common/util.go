package common

import (
	"html/template"
	"net/url"
	"regexp"
	"strings"
)

var (
	rxSan        = regexp.MustCompile(`(?m)(\n|\[img\]https?://\S+?\[/img\]|\[code\][\s\S]+?\[/code\]|<|https?://[^\s<>"'#\[\]]+|@\S+|#\S+)|\[mj\]\d+\[/mj\]|\[enc\]\w+\[/enc\]`)
	rxFirstImage = regexp.MustCompile(`(?i)(https?://\S+\.(png|jpg|gif|webp|jpeg)|\[img\]https?://\S+\[/img\])`)
	rxMentions   = regexp.MustCompile(`((@|#)\S+)`)

	ReverseTemplateRenderFunc func(string, interface{}) string
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
		if strings.HasPrefix(in, "[img]") && strings.HasSuffix(in, "[/img]") {
			return "<a href='" + in[5:len(in)-6] + "' target=_blank>" + in[5:len(in)-6] + "</a>"
		}
		if strings.HasPrefix(in, "[mj]") {
			return "<img class=majiang src='https://static.saraba1st.com/image/smiley/face2017/" + in[4:len(in)-5] + ".png'>"
		}
		if strings.HasPrefix(in, "[enc]") {
			return strings.TrimSpace(ReverseTemplateRenderFunc("enc.html", in[5:len(in)-6]))
		}
		if len(in) > 0 {
			s := SafeStringForCompressString(template.HTMLEscapeString(in[1:]))
			if in[0] == '#' {
				AddTagToSearch(in[1:])
				return "<a href='/tag/" + s + "' target=_blank>" + in + "</a>"
			}
			if in[0] == '@' {
				AddUserToSearch(in[1:])
				return "<a href='javascript:void(0)'" +
					` class=mentioned-user` +
					` onclick="showInfoBox(this,'` + in[1:] + `')">` +
					in + "</a>"
			}
		}
		return "<a href='" + in + "' target=_blank>" + in + "</a>"
	})
	return in
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
