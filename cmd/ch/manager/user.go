package manager

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
	return m.db.Set("\x00ban"+id, []byte{1})
}

func (m *Manager) Unban(id string) error {
	return m.db.Delete("\x00ban" + id)
}

func (m *Manager) IsBanned(id string) bool {
	x := m.kvMustGet("\x00ban" + id)
	return len(x) == 1 && x[0] == 1
}

func (m *Manager) UserExisted(id string) (ok bool) {
	return true
}
