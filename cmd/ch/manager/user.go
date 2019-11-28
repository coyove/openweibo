package manager

import (
	"bytes"
	"encoding/base64"
	"fmt"

	"github.com/coyove/iis/cmd/ch/config"
	mv "github.com/coyove/iis/cmd/ch/model"
)

func (m *Manager) GetUser(id string) (*mv.User, error) {
	return mv.UnmarshalUser(m.kvMustGet("u/" + id))
}

func (m *Manager) GetUserByToken(tok string) (*mv.User, error) {
	x, err := base64.StdEncoding.DecodeString(tok)
	if err != nil {
		return nil, err
	}

	pos := []int{} //TODO
	for i := 0; i < len(x)-16; i += 4 {
		pos = append(pos, i)
	}

	for i := len(pos) - 1; i >= 0; i-- {
		config.Cfg.Blk.Decrypt(x[pos[i]:], x[pos[i]:])
	}

	idx := bytes.IndexByte(x, 0)
	if idx == -1 {
		return nil, fmt.Errorf("invalid token format")
	}

	id, session := x[:idx], x[idx+1:]
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

func (m *Manager) BanUser(id string, ban bool) error {
	u, err := m.GetUser(id)
	if err != nil {
		return err
	}
	u.Banned = ban
	return m.SetUser(u)
}

func (m *Manager) SetUser(u *mv.User) error {
	return m.db.Set("u/"+u.ID, u.Marshal())
}

func (m *Manager) LockUserID(id string) {
	m.db.Lock(id)
}

func (m *Manager) UnlockUserID(id string) {
	m.db.Unlock(id)
}
