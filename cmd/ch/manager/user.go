package manager

import (
	"log"

	"github.com/coyove/iis/cmd/ch/ident"
	"github.com/etcd-io/bbolt"
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
	return m.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(bkPost).Put([]byte("\x00ban"+id), []byte{1})
	})
}

func (m *Manager) Unban(id string) error {
	return m.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(bkPost).Delete([]byte("\x00ban" + id))
	})
}

func (m *Manager) IsBanned(bk *bbolt.Bucket, id string) bool {
	if bk == nil {
		r := false
		m.db.View(func(tx *bbolt.Tx) error {
			r = m.IsBanned(tx.Bucket(bkPost), id)
			return nil
		})
		return r
	}
	return len(bk.Get([]byte("\x00ban"+id))) > 0
}

func (m *Manager) UserExisted(id string) (ok bool) {
	m.db.View(func(tx *bbolt.Tx) error {
		c := tx.Bucket(bkPost).Cursor()
		k, _ := cursorMoveToLast(c, id)

		if k == nil {
			return nil
		}

		kid := ident.ParseID(k)
		log.Println(kid.Tag(), id)

		ok = kid.Header() == ident.HeaderAuthorTag && kid.Tag() == id
		return nil
	})
	return
}
