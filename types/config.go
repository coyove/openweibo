package types

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/coyove/sdss/contrib/skip32"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

var Config struct {
	Key            string
	ImageCacheSize int
	S3             struct {
		Endpoint  string
		Region    string
		AccessKey string
		SecretKey string
	}
	RootPassword string
	Runtime      struct {
		AESBlock cipher.Block
		Skip32   skip32.Skip32
	} `json:"-"`
}

func LoadConfig(path string) {
	buf, err := ioutil.ReadFile(path)
	if err != nil {
		logrus.Fatal("load config: ", err)
	}
	if err := json.Unmarshal(buf, &Config); err != nil {
		logrus.Fatal("load config unmarshal: ", err)
	}

	for ; len(Config.Key) < 48; Config.Key += "0123456789abcdef" {
	}

	Config.Runtime.AESBlock, err = aes.NewCipher([]byte(Config.Key[16:48]))
	if err != nil {
		logrus.Fatal("load config cipher key: ", err)
	}

	Config.Runtime.Skip32 = skip32.ReadSkip32Key(Config.Key[:10])

	{
		tmp, _ := json.Marshal(Config)
		logrus.Infof("load config: %v", string(tmp))
	}
}

type Request struct {
	*http.Request
	Start       time.Time
	ServerStart time.Time
	T           map[string]interface{}

	P struct {
		built    bool
		Page     int
		Desc     bool
		PageSize int
		Sort     int
		uq       url.Values
	}

	ServeUUID   string
	Config      func() gjson.Result
	User        UserHash
	UserDisplay string
	UserSession string
	RemoteIPv4  net.IP
}

func (r *Request) GenerateSession(name byte) (UserHash, string) {
	uh, h := r.newUserHash(name)
	nonce := [12]byte{}
	rand.Read(nonce[:])
	aesgcm, _ := cipher.NewGCM(Config.Runtime.AESBlock)
	return uh, base64.URLEncoding.EncodeToString(append(nonce[:], aesgcm.Seal(nil, nonce[:], h, nil)...))
}

func (r *Request) parseSession(v string) (UserHash, bool) {
	buf, err := base64.URLEncoding.DecodeString(v)
	if err != nil || len(buf) < 12 {
		return UserHash{}, false
	}
	aesgcm, _ := cipher.NewGCM(Config.Runtime.AESBlock)
	data, err := aesgcm.Open(nil, buf[:12], buf[12:], nil)
	if err != nil {
		return UserHash{}, false
	}
	return ParseUserHash(data), true
}

func (r *Request) ParseSession() (string, bool) {
	if sess, _ := r.Cookie("session"); sess != nil {
		uh, ok := r.parseSession(sess.Value)
		if ok {
			r.User = uh
			r.UserDisplay = uh.Display()
			r.UserSession = sess.Value
			return sess.Value, false
		}
	}
	uh, s := r.GenerateSession(0)

	// fmt.Println(uh, r.UserAgent())
	// fmt.Println(ParseUserHash(uh.base64))

	r.User = uh
	r.UserDisplay = uh.Display()
	r.UserSession = s
	return s, true
}

func (r *Request) AddTemplateValue(k string, v interface{}) {
	if r.T == nil {
		r.T = map[string]interface{}{}
	}
	r.T[k] = v
}

func (r *Request) Elapsed() int64 {
	return int64(time.Since(r.Start).Milliseconds())
}

func (r *Request) ParsePaging() url.Values {
	if r.P.built {
		return r.P.uq
	}

	uq := r.URL.Query()
	p, _ := strconv.Atoi(uq.Get("p"))
	if p < 1 {
		p = 1
	}
	sort, _ := strconv.Atoi(uq.Get("sort"))
	if sort < -1 || sort > 1 {
		sort = 0
	}
	desc := uq.Get("desc") == "1" || uq.Get("desc") == "true"
	pageSize, _ := strconv.Atoi(uq.Get("pagesize"))
	if pageSize <= 0 {
		pageSize = 28
	}
	if pageSize > 50 {
		pageSize = 50
	}

	r.P.Page, r.P.Sort, r.P.Desc, r.P.PageSize = p, sort, desc, pageSize
	r.P.uq = uq
	r.P.built = true
	return uq
}

func (r *Request) BuildPageLink(p int) string {
	return fmt.Sprintf("p=%d&sort=%d&desc=%v&pagesize=%d", p,
		r.P.Sort, r.P.Desc, r.P.PageSize)
}

func (r *Request) RemoteIPv4Masked() net.IP {
	ip := append(net.IP{}, r.RemoteIPv4...)
	ip[3] = 0
	return ip
}

func (r *Request) GetTitleMaxLen() int {
	v := r.Config().Get("title_max").Int()
	if v <= 0 {
		return 80
	}
	return int(v)
}

func (r *Request) GetContentMaxLen() int {
	v := r.Config().Get("content_max").Int()
	if v <= 0 {
		return 500000
	}
	return int(v)
}

func (r *Request) GetParentsMax() int {
	v := r.Config().Get("parents_max").Int()
	if v <= 0 {
		return 32
	}
	return int(v)
}
