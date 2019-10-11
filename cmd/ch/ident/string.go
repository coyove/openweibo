package ident

import "strings"

const (
	prefix = "0000000000000000"
	zeros  = prefix + prefix
)

func BytesString(p []byte) string {
	x := EncryptArticleID(p)
	return strings.Replace(x, prefix, ".", 1)
}

func BytesPlainString(p []byte) string {
	x := idEncoding.EncodeToString(p)
	return strings.Replace(x, prefix, ".", 1)
}

func StringBytes(s string) []byte {
	if len(s) < 10 {
		return nil
	}
	s = strings.Replace(s, ".", prefix, 1)
	return DecryptArticleID(s)
}
