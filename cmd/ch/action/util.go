package action

import (
	"fmt"
	"net/url"
	"unicode"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/ident"
	mv "github.com/coyove/iis/cmd/ch/model"
	"github.com/gin-gonic/gin"
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

func sanUsername(id string) string {
	buf := []byte(id)
	for i, c := range buf {
		if c := rune(c); !unicode.IsDigit(c) && !unicode.IsLetter(c) {
			buf[i] = '_'
		}
		if i == 12 {
			buf = buf[:12]
			break
		}
	}
	return string(buf)
}
