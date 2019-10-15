package ident

import (
	"github.com/coyove/iis/cmd/ch/captcha"
	"github.com/coyove/iis/cmd/ch/config"
)

func GenerateCaptcha(buf [4]byte) string {
	return captcha.NewImage(config.Cfg.Key, buf[:], 120, 50).PNGBase64()
}
