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
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"golang.org/x/crypto/xtea"
)

var Config struct {
	Key      string
	DynamoDB struct {
		Region    string
		AccessKey string
		SecretKey string
	}
	Runtime struct {
		AESBlock cipher.Block
		XTEA     *xtea.Cipher
	}
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

	Config.Runtime.XTEA, err = xtea.NewCipher([]byte(Config.Key[:16]))
	if err != nil {
		logrus.Fatal("load config xtea cipher key: ", err)
	}
}

type Request struct {
	*http.Request
	Start       time.Time
	ServerStart time.Time
	T           map[string]interface{}

	paging struct {
		built    bool
		current  int
		desc     bool
		pageSize int
		sort     int
	}

	ServeUUID   string
	Config      func() gjson.Result
	User        UserHash
	UserDisplay string
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
			return sess.Value, false
		}
	}
	uh, s := r.GenerateSession(0)

	// fmt.Println(uh, r.UserAgent())
	// fmt.Println(ParseUserHash(uh.base64))

	r.User = uh
	r.UserDisplay = uh.Display()
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

func (r *Request) GetPagingArgs() (int, int, bool, int) {
	if r.paging.built {
		return r.paging.current, r.paging.sort, r.paging.desc, r.paging.pageSize
	}

	p, _ := strconv.Atoi(r.URL.Query().Get("p"))
	if p < 1 {
		p = 1
	}
	sort, _ := strconv.Atoi(r.URL.Query().Get("sort"))
	if sort < -1 || sort > 1 {
		sort = 0
	}
	desc := r.URL.Query().Get("desc") == "1"
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pagesize"))
	if pageSize <= 0 {
		pageSize = 30
	}
	if pageSize > 30 {
		pageSize = 30
	}

	r.paging.current, r.paging.sort, r.paging.desc, r.paging.pageSize = p, sort, desc, pageSize
	r.paging.built = true

	r.AddTemplateValue("page", p)
	r.AddTemplateValue("sort", sort)
	r.AddTemplateValue("desc", desc)
	r.AddTemplateValue("pageSize", pageSize)
	return p, sort, desc, pageSize
}

func (r *Request) BuildPageLink(p int) string {
	_, sort, desc, pageSize := r.GetPagingArgs()
	d := "1"
	if !desc {
		d = ""
	}
	return fmt.Sprintf("p=%d&sort=%d&desc=%v&pagesize=%d", p, sort, d, pageSize)
}

func (r *Request) RemoteIPv4Masked() net.IP {
	ip := append(net.IP{}, r.RemoteIPv4...)
	ip[3] = 0
	return ip
}

func (r *Request) GetTitleMaxLen() int {
	v := r.Config().Get("title_max").Int()
	if v <= 0 {
		return 50
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
		return 8
	}
	return int(v)
}
