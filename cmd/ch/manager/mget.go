package manager

import (
	"github.com/coyove/iis/cmd/ch/ident"
	mv "github.com/coyove/iis/cmd/ch/model"
)

func (m *Manager) GetReplies(parent string, start, end int) (a []*mv.Article) {
	for i := start; i < end; i++ {
		if i <= 0 || i >= 128*128 {
			continue
		}

		pid := ident.ParseIDString(nil, parent)
		if !pid.RIndexAppend(int16(i)) {
			break
		}

		p, _ := m.Get(pid.String())
		if p == nil {
			p = &mv.Article{ID: pid.String()}
		}
		a = append(a, p)
	}
	return
}
