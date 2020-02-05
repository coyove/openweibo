package common

import (
	"encoding/base64"
	"strconv"
	"strings"
)

func Pack256(m map[string]string) string {
	x := [32]byte{}
	for k, v := range m {
		i, _ := strconv.Atoi(k)
		if i < 0 || i > 255 {
			panic(i)
		}
		if v != "1" {
			continue
		}
		idx, bidx := i/8, i%8
		x[idx] |= 1 << bidx
	}
	return strings.TrimRight(base64.StdEncoding.EncodeToString(x[:]), "=")
}

func Unpack256(v string) map[string]string {
	x, _ := base64.StdEncoding.DecodeString(v + "=")
	if len(x) != 32 {
		return nil
	}
	m := map[string]string{}
	for i, v := range x {
		for j := 0; j < 8; j++ {
			z := i*8 + j
			if (v>>j)&1 == 1 {
				m[strconv.Itoa(z)] = "1"
			}
		}
	}
	return m
}
