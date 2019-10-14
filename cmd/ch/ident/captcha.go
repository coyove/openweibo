package ident

import (
	"bytes"
	"encoding/base64"
	"sync"

	"github.com/coyove/iis/cmd/ch/captcha"
	"github.com/coyove/iis/cmd/ch/config"
)

var bytesPool = sync.Pool{
	New: func() interface{} {
		return &bytes.Buffer{}
	},
}

func GenerateCaptcha(buf [4]byte) string {
	b := bytesPool.Get().(*bytes.Buffer)
	captcha.NewImage(config.Cfg.Key, buf[:], 120, 60).WriteTo(b)
	ret := base64.StdEncoding.EncodeToString(b.Bytes())
	b.Reset()
	bytesPool.Put(b)
	return ret
}
