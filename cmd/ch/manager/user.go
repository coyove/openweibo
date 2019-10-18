package manager

import (
	"github.com/coyove/iis/cmd/ch/ident"
	mv "github.com/coyove/iis/cmd/ch/model"
)

//type User struct {
//	ID           string `protobuf:"bytes,1,opt"`
//	Banned       bool   `protobuf:"varint,7,opt"`
//	WechatOpenID string `protobuf:"bytes,2,opt"`
//}
//
//func (a *User) Reset() { *a = User{} }
//
//func (a *User) String() string { return proto.CompactTextString(a) }
//
//func (a *User) ProtoMessage() {}
//
//func (u *User) unmarshal(p []byte) error {
//	return proto.Unmarshal(p, u)
//}
//
//func (u *User) marshal() []byte {
//	p, _ := proto.Marshal(u)
//	return p
//}
//
//func userKey(id string) []byte {
//	return []byte("\x00u_" + id)
//}

func (m *Manager) Ban(id string) error {
	return m.db.Set("x-ban-"+id, []byte("true"))
}

func (m *Manager) Unban(id string) error {
	return m.db.Delete("x-ban-" + id)
}

func (m *Manager) IsBanned(id string) bool {
	x := m.kvMustGet("x-ban-" + id)
	return string(x) == "true"
}

func (m *Manager) UserExisted(tok string) (ok bool) {
	id := ident.NewTagID(tok).String()
	tl, _ := mv.UnmarshalTimeline(m.kvMustGet(id))
	return tl != nil && tl.Next != ""
}
