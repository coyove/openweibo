package mv

import (
	"html/template"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/coyove/iis/cmd/ch/config"
)

var (
	rxSan        = regexp.MustCompile(`(?m)(^>.+|<|https?://[^\s<>"'#]+|@\S+|#\S+)`)
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
		if len(in) > 0 {
			s := SafeStringForCompressString(template.HTMLEscapeString(in[1:]))
			if in[0] == '#' {
				return "<a href='/tag/" + s + "'>" + in + "</a>"
			}
			if in[0] == '@' {
				return "<a href='/t/" + s + "'>" + in + "</a>"
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
		return m[0]
	}
	return ""
}

var usCache [65536][16]rune

func SearchUsers(id string, n int) []string {
	m := map[uint32]bool{}
	idr := []rune(id)
	rtos := func(r [16]rune) string {
		for j, c := range r {
			if c == 0 {
				return string(r[:j])
			}
		}
		return string(r[:])
	}

	if len(idr) == 1 {
		res := make([]string, 0, n)
		for _, id := range usCache {
			if id == [16]rune{} {
				continue
			}
			for _, r := range id {
				if r == idr[0] {
					res = append(res, rtos(id))
					break
				}
			}
			if len(res) == n {
				break
			}
		}
		return res
	}

	if len(idr) < 2 {
		return nil
	}

	lower := func(r rune) uint32 {
		if r >= 'A' && r <= 'Z' {
			r = r - 'A' + 'a'
		}
		return uint32(uint16(r))
	}

	for i := 0; i < len(idr)-1; i++ {
		a, b := lower(idr[i]), lower(idr[i+1])
		m[a<<16|b] = true
	}

	bigram := func(a [16]rune) int {
		s := 0
		for i := 0; i < len(a)-1; i++ {
			v := lower(a[i])<<16 | lower(a[i+1])
			if v == 0 {
				break
			}
			if m[v] {
				s++
			}
		}
		return s
	}

	scores := make([]struct {
		res   [16]rune
		score int
	}, n)

	for _, id := range usCache {
		if id == [16]rune{} {
			continue
		}
		s := bigram(id)
		i := sort.Search(n, func(i int) bool {
			return scores[i].score >= s
		})
		if i < len(scores) {
			scores = append(scores[:i+1], scores[i:]...)
		} else {
			scores = append(scores, scores[0])
		}
		scores[i].res = id
		scores[i].score = s
		scores = scores[1:]
	}

	res := make([]string, 0, n)
	for i := len(scores) - 1; i >= 0; i-- {
		if scores[i].score == 0 || scores[i].res == [16]rune{} {
			break
		}
		res = append(res, rtos(scores[i].res))
	}
	return res
}
