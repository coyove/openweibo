package ident

import "strings"

const (
	prefix = "0000000000000000"
	zeros  = prefix + prefix
)

func BytesString(p []byte) string {
	x := idEncoding.EncodeToString(p)
	return strings.Replace(strings.TrimRight(x, "0"), prefix, ".", 1)
}

func StringBytes(s string) []byte {
	if len(s) < 10 {
		return nil
	}
	s = strings.Replace(s, ".", prefix, 1)
	if len(s) < 40 {
		s = s + zeros[:40-len(s)]
	}
	b, _ := idEncoding.DecodeString(s)
	return b
}
