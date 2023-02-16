package types

import (
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"net/http"

	"github.com/coyove/sdss/contrib/clock"
	"github.com/gogo/protobuf/proto"
)

type User struct {
	Id           string `protobuf:"bytes,1,opt"`
	PwdHash      []byte `protobuf:"bytes,2,opt"`
	Email        string `protobuf:"bytes,3,opt"`
	CreateUA     string `protobuf:"bytes,4,opt"`
	CreateIP     string `protobuf:"bytes,5,opt"`
	LoginUA      string `protobuf:"bytes,6,opt"`
	LoginIP      string `protobuf:"bytes,7,opt"`
	Session64    int64  `protobuf:"fixed64,8,opt"`
	RoleInt      int64  `protobuf:"varint,9,opt"`
	CreateUnix   int64  `protobuf:"fixed64,10,opt"`
	LoginUnix    int64  `protobuf:"fixed64,12,opt"`
	UploadSize   int64  `protobuf:"fixed64,11,opt"`
	LastResetPwd string `protobuf:"bytes,100,opt"`
	HideImage    bool   `protobuf:"varint,101,opt"`
}

func (t *User) Reset() { *t = User{} }

func (t *User) ProtoMessage() {}

func (t *User) String() string { return proto.CompactTextString(t) }

func (t *User) Valid() bool {
	return t != nil && t.Id != ""
}

func (t *User) IsMod() bool {
	return t != nil && (t.Id == "root" || t.RoleInt == 1)
}

func (t *User) IsRoot() bool {
	return t != nil && (t.Id == "root")
}

func (t *User) Role() string {
	if t.IsRoot() {
		return "root"
	}
	if t.IsMod() {
		return "mod"
	}
	return "user"
}

func (t *User) MarshalBinary() []byte {
	buf, err := proto.Marshal(t)
	if err != nil {
		panic(err)
	}
	return buf
}

func UnmarshalUserBinary(p []byte) *User {
	t := &User{}
	if err := proto.Unmarshal(p, t); err != nil {
		return &User{}
	}
	return t
}

func (u *User) GenerateSession() *http.Cookie {
	t := *u
	t.CreateIP = ""
	t.CreateUA = ""
	t.LoginIP = ""
	t.LoginUA = ""
	t.LastResetPwd = ""

	h := append(t.MarshalBinary(), 0, 0, 0, 0)
	x := uint32(clock.Unix() + Config.SessionTTL)
	binary.BigEndian.PutUint32(h[len(h)-4:], x)
	nonce := [12]byte{}
	rand.Read(nonce[:])
	aesgcm, _ := cipher.NewGCM(Config.Runtime.AESBlock)
	data := append(nonce[:], aesgcm.Seal(nil, nonce[:], h, nil)...)
	return &http.Cookie{
		Name:   "session",
		Value:  base64.URLEncoding.EncodeToString(data),
		Path:   "/",
		MaxAge: int(Config.SessionAge),
	}
}
