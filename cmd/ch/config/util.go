package config

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"unicode"
)

func HashName(n string) string {
	n0 := make([]byte, 12)
	copy(n0, "com.")

	for i := 0; i < len(n) && i < len(n0); i++ {
		if !unicode.IsLetter(rune(n[i])) && !unicode.IsDigit(rune(n[i])) {
			break
		}
		n0[i] = n[i]
	}

	n0[3] = '.'

	if len(n) < 6 {
		rand.Read(n0[:])
		return base64.URLEncoding.EncodeToString(n0[:6])
	}

	h := hmac.New(sha1.New, Cfg.KeyBytes)
	h.Write([]byte(n))
	x := h.Sum(nil)

	base64.URLEncoding.Encode(n0[4:], x[:6])
	return string(n0)
}
