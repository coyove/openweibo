package types

import (
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/coyove/sdss/contrib/clock"
	"github.com/tidwall/gjson"
)

var userHashBase64 = base64.NewEncoding("-0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdefghijklmnopqrstuvwxyz")

type Request struct {
	*http.Request
	Start       int64
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
	return (clock.UnixNano() - r.Start) / 1e6
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

var errorMessages = map[string]string{
	"INTERNAL_ERROR":     "服务器错误",
	"IP_BANNED":          "IP封禁",
	"MODS_REQUIRED":      "无管理员权限",
	"PENDING_REVIEW":     "修改审核中",
	"LOCKED":             "记事已锁定",
	"INVALID_CONTENT":    "无效内容，过长或过短",
	"EMPTY_TITLE":        "标题为空，请输入标题或者选择一篇父记事",
	"TITLE_TOO_LONG":     "标题过长",
	"CONTENT_TOO_LONG":   "内容过长",
	"TOO_MANY_PARENTS":   "父记事过多，最多8个",
	"DUPLICATED_TITLE":   "标题重名",
	"ILLEGAL_APPROVE":    "无权审核",
	"INVALID_ACTION":     "请求错误",
	"INVALID_IMAGE_NAME": "无效图片名",
	"INVALID_IMAGE":      "无效图片",
	"INVALID_PARENT":     "无效父记事",
	"CONTENT_TOO_LARGE":  "图片过大",
	"COOLDOWN":           "请稍后重试",
	"CANT_TOUCH_SELF":    "无法收藏自己的记事",
	"DATA_NO_CHANGE":     "请编辑记事",
}

type Response struct {
	Written int64
	http.ResponseWriter
}

func (w *Response) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	w.Written += int64(n)
	return n, err
}

func (w *Response) WriteJSON(args ...interface{}) {
	m := map[string]interface{}{}
	for i := 0; i < len(args); i += 2 {
		k, v := args[i].(string), args[i+1]
		if k == "code" {
			m["msg"] = errorMessages[v.(string)]
		}
		m[k] = v
	}
	buf, _ := json.Marshal(m)
	w.Header().Add("Content-Type", "application/json")
	w.Write(buf)
}

func uaHash(s string) uint64 {
	const offset64 = 14695981039346656037
	const prime64 = 1099511628211
	var hash uint64 = offset64
	for i := 0; i < len(s); i++ {
		if s[i] == '.' || s[i] == ' ' || (s[i] >= '0' && s[i] <= '9') {
			hash *= prime64
			hash ^= uint64(s[i])
		}
	}
	return uint64(hash)
}

func (r *Request) newUserHash(name byte) (UserHash, []byte) {
	tmp := make([]byte, 6+8)
	enc := tmp[6:]
	tmp = tmp[:6]

	ip := r.RemoteIPv4
	copy(tmp[:3], ip)
	hr := clock.Unix() / 3600
	binary.BigEndian.PutUint16(tmp[3:], uint16(uaHash(r.UserAgent()))*24+uint16(hr%24))
	tmp[5] = name

	Config.Runtime.Skip32.EncryptBytes(tmp[2:])
	Config.Runtime.Skip32.EncryptBytes(tmp[:4])

	userHashBase64.Encode(enc, tmp)
	return UserHash{
		IP:     ip,
		Name:   name,
		base64: enc,
	}, enc
}

type UserHash struct {
	IP     net.IP
	Name   byte
	base64 []byte
}

func (uh UserHash) IsRoot() bool {
	return uh.Name == 'r'
}

func (uh UserHash) IsMod() bool {
	return uh.Name == 'r' || uh.Name == 'm'
}

func (uh UserHash) Display() string {
	if uh.Name == 'r' {
		return "root"
	}
	if uh.Name == 'm' {
		return "mod." + string(uh.base64)
	}
	return string(uh.base64)
}

func ParseUserHash(v []byte) UserHash {
	tmp := make([]byte, 6)
	if len(v) >= 8 {
		userHashBase64.Decode(tmp, v[:8])
	}

	Config.Runtime.Skip32.DecryptBytes(tmp[:4])
	Config.Runtime.Skip32.DecryptBytes(tmp[2:])

	ip := net.IP(tmp[:4])
	ip[3] = 0
	name := tmp[5]

	return UserHash{ip, name, v}
}
