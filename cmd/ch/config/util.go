package config

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"unicode"
)

func HashName(n string) string {
	n0 := make([]byte, 11)
	copy(n0, "com")

	for i := 0; i < len(n) && i < len(n0); i++ {
		if !unicode.IsLetter(rune(n[i])) && !unicode.IsDigit(rune(n[i])) {
			break
		}
		n0[i] = n[i]
	}

	n0[3] = '.'

	h := hmac.New(sha1.New, Cfg.KeyBytes)
	if len(n) == 0 {
		copy(n0, "nan.")
		rand.Read(n0[4:8])
		h.Write(n0[4:8])
	} else {
		h.Write([]byte(n))
	}
	h.Write(Cfg.KeyBytes)
	x := h.Sum(nil)

	base64.URLEncoding.Encode(n0[4:], x[:3])
	base64.URLEncoding.Encode(n0[7:], x[3:6])
	return string(n0)
}
