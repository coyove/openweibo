package types

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"io/ioutil"
	"reflect"

	"github.com/coyove/sdss/contrib/skip32"
	"github.com/sirupsen/logrus"
)

var Config struct {
	Key            string
	Domain         string
	ImageDomain    string
	PagingTimeout  int64
	RootPassword   string
	RequestMaxSize int64
	TzOffset       int64

	ImageCache struct {
		MaxFiles       int64
		PurgerInterval int64
	}

	Index struct {
		SwitchThreshold int64
		CacheSize       int64
	}

	S3 struct {
		Endpoint  string
		Region    string
		AccessKey string
		SecretKey string
	}

	Runtime struct {
		AESBlock cipher.Block
		Skip32   skip32.Skip32
	}
}

func LoadConfig(path string) {
	ifZero := func(v *int64, d, scale int64) {
		if *v <= 0 {
			*v = d
		}
		*v *= scale
	}

	buf, err := ioutil.ReadFile(path)
	if err != nil {
		logrus.Fatal("load config: ", err)
	}
	if err := json.Unmarshal(buf, &Config); err != nil {
		logrus.Fatal("load config unmarshal: ", err)
	}

	if Config.RootPassword == "" {
		logrus.Fatal("load config: missing RootPassword")
	}

	ifZero(&Config.TzOffset, 8, 1)
	ifZero(&Config.RequestMaxSize, 15, 1024*1024)
	ifZero(&Config.ImageCache.MaxFiles, 1000, 1)
	ifZero(&Config.ImageCache.PurgerInterval, 10, 1e9)
	ifZero(&Config.Index.CacheSize, 1024, 1024*1024)
	ifZero(&Config.Index.SwitchThreshold, 1024000, 1)
	ifZero(&Config.PagingTimeout, 2000, 1e6)

	for ; len(Config.Key) < 48; Config.Key += "0123456789abcdef" {
	}

	Config.Runtime.AESBlock, err = aes.NewCipher([]byte(Config.Key[16:48]))
	if err != nil {
		logrus.Fatal("load config cipher key: ", err)
	}

	Config.Runtime.Skip32 = skip32.ReadSkip32Key(Config.Key[:10])

	rv := reflect.ValueOf(Config)
	for i := 0; i < rv.NumField(); i++ {
		n := rv.Type().Field(i).Name
		if n == "Runtime" {
			break
		}
		f := rv.Field(i)
		if f.Kind() == reflect.Struct {
			for i := 0; i < f.NumField(); i++ {
				tmp, _ := json.Marshal(f.Field(i).Interface())
				logrus.Infof("[Config] %s.%s=%s", n, f.Type().Field(i).Name, tmp)
			}
		} else {
			tmp, _ := json.Marshal(f.Interface())
			logrus.Infof("[Config] %s=%s", n, tmp)
		}
	}
}
