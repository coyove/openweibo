package manager

import (
	"bytes"
	"encoding/base64"
	"fmt"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/ident"
	mv "github.com/coyove/iis/cmd/ch/model"
)

func (m *Manager) GetUser(id string) (*mv.User, error) {
	return mv.UnmarshalUser(m.kvMustGet("u/" + id))
}

func (m *Manager) GetUserByToken(tok string) (*mv.User, error) {
	if tok == "" {
		return nil, fmt.Errorf("invalid token")
	}

	x, err := base64.StdEncoding.DecodeString(tok)
	if err != nil {
		return nil, err
	}

	for i := len(x) - 16; i >= 0; i -= 8 {
		config.Cfg.Blk.Decrypt(x[i:], x[i:])
	}

	parts := bytes.SplitN(x, []byte("\x00"), 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	session, id := parts[0], parts[1]
	u, err := m.GetUser(string(id))
	if err != nil {
		return nil, err
	}

	if u.Session != string(session) {
		return nil, fmt.Errorf("invalid token session")
	}
	return u, nil
}

func (m *Manager) IsBanned(id string) bool {
	u, err := m.GetUser(id)
	if err != nil {
		return true
	}
	return u.Banned
}

func (m *Manager) SetUser(u *mv.User) error {
	if u.ID == "" {
		return nil
	}
	return m.db.Set("u/"+u.ID, u.Marshal())
}

func (m *Manager) LockUserID(id string) {
	m.db.Lock(id)
}

func (m *Manager) UnlockUserID(id string) {
	m.db.Unlock(id)
}

func (m *Manager) UpdateUser(id string, cb func(u *mv.User) error) error {
	m.db.Lock(id)
	defer m.db.Unlock(id)
	return m.UpdateUser_unlock(id, cb)
}

func (m *Manager) UpdateUser_unlock(id string, cb func(u *mv.User) error) error {
	u, err := m.GetUser(id)
	if err != nil {
		return err
	}
	if err := cb(u); err != nil {
		return err
	}
	return m.SetUser(u)
}

func (m *Manager) MentionUser(a *mv.Article, id string) error {
	if err := m.insertTag(a.ID, ident.IDTagInbox, id); err != nil {
		return err
	}
	return m.UpdateUser(id, func(u *mv.User) error {
		u.Unread++
		return nil
	})
}
