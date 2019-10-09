package token

import (
	"bytes"
	"encoding/base64"
	"sync"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/dchest/captcha"
)

var bytesPool = sync.Pool{
	New: func() interface{} {
		return &bytes.Buffer{}
	},
}

func GenerateCaptcha(buf [6]byte) string {
	b := bytesPool.Get().(*bytes.Buffer)
	captcha.NewImage(config.Cfg.Key, buf[:6], 180, 60).WriteTo(b)
	ret := base64.StdEncoding.EncodeToString(b.Bytes())
	b.Reset()
	bytesPool.Put(b)
	return ret
}
