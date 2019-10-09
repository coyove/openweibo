package action

import (
	"net/url"

	"github.com/coyove/iis/cmd/ch/config"
)

func checkCategory(u string) string {
	if u == "default" {
		return u
	}
	if !config.Cfg.TagsMap[u] {
		return "default"
	}
	return u
}

func encodeQuery(a ...string) string {
	query := url.Values{}
	for i := 0; i < len(a); i += 2 {
		if a[i] != "" {
			query.Add(a[i], a[i+1])
		}
	}
	return query.Encode()
}
