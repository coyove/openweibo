package mv

import (
	"regexp"
	"strings"
	"time"
)

var rxSan = regexp.MustCompile(`(?m)(^>.+|<|https?://\S+)`)

func FormatTime(x time.Time, sec bool) string {
	now := time.Now().UTC().Add(8 * time.Hour)
	if now.YearDay() == x.YearDay() && now.Year() == x.Year() {
		return x.Format("15:04:05")
	}
	return x.Format("2006-01-02T15:04:05")
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
	return rxSan.ReplaceAllStringFunc(in, func(in string) string {
		if in == "<" {
			return "&lt;"
		}
		if strings.HasPrefix(in, ">") {
			return "<code>" + strings.TrimSpace(in[1:]) + "</code>"
		}
		return "<a href='" + in + "' target=_blank>" + in + "</a>"
	})
}
