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
	"strings"
	"time"

	"github.com/sirupsen/logrus"
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

	Config.Runtime.AESBlock, err = aes.NewCipher([]byte(Config.Key))
	if err != nil {
		logrus.Fatal("load config cipher key: ", err)
	}
}

type Request struct {
	*http.Request
	Start time.Time
	T     map[string]interface{}

	paging struct {
		built    bool
		current  int
		desc     bool
		pageSize int
		sort     int
	}

	User        UserHash
	UserDisplay string
}

func (r *Request) GenerateSession(name string) (UserHash, string) {
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
	uh, s := r.GenerateSession("")
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
		pageSize = 50
	}
	if pageSize > 50 {
		pageSize = 50
	}

	r.paging.current, r.paging.sort, r.paging.desc, r.paging.pageSize = p, sort, desc, pageSize
	r.paging.built = true
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
	ip := r.RemoteIPv4()
	ip[3] = 0
	return ip
}

func (r *Request) RemoteIPv4() net.IP {
	xff := r.Header.Get("X-Forwarded-For")
	ips := strings.Split(xff, ",")
	for _, ip := range ips {
		p := net.ParseIP(strings.TrimSpace(ip))
		if p != nil {
			return p.To4()
		}
		break
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	p := net.ParseIP(ip)
	if p != nil {
		return p.To4()
	}
	return net.IP{0, 0, 0, 0}
}
