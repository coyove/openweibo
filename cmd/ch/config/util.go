package config

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"strings"
)

func HashName(n string) string {
	var n0 string
	if len(n) >= 4 {
		n0 = strings.TrimLeft(n[:4], "#")
		for i := 0; i < len(n0); i++ {
			if n0[i] > 127 {
				n0 = n0[:i]
				break
			}
		}
		n = n[4:]
	}

	h := hmac.New(sha1.New, []byte(Cfg.Key))
	h.Write([]byte(n + Cfg.Key))
	x := h.Sum(nil)
	return n0 + base64.URLEncoding.EncodeToString(x[:6])
}
