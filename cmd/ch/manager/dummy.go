package manager

import (
	"strings"
	"time"

	"github.com/coyove/iis/cmd/ch/ident"
	mv "github.com/coyove/iis/cmd/ch/model"
)

func (m *Manager) insertTagAndUpdateArticleBuggy(a *mv.Article, tag string) error {
	if err := m.insertTagAndUpdateArticleBuggy_(a, tag); err != nil {
		return err
	}

	time.Sleep(time.Second * 2)

	return m.db.Set(a.ID, a.Marshal())
}

func (m *Manager) insertTagAndUpdateArticleBuggy_(a *mv.Article, tag string) error {
	rootID := ident.NewTagID(tag).String()

	m.db.Lock(rootID)
	defer m.db.Unlock(rootID)

	root, err := mv.UnmarshalTimeline(m.kvMustGet(rootID))
	if err != nil && strings.Contains(err.Error(), "#ERR") {
		return err
	}

	if root == nil {
		root = &mv.Timeline{
			ID: rootID,
		}
	}

	a.TimelineID = ident.NewID().String()

	tl := &mv.Timeline{
		ID:   a.TimelineID,
		Ptr:  a.ID,
		Next: root.Next,
	}

	if err := m.db.Set(tl.ID, tl.Marshal()); err != nil {
		return err
	}

	root.Next = tl.ID
	if err := m.db.Set(root.ID, root.Marshal()); err != nil {
		// If failed, the newly created timeline marker 'tl' will be left in the database without being used ever again
		return err
	}

	return nil

}
