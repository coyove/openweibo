package manager

//
// import (
// 	"fmt"
// 	"strconv"
// 	"time"
//
// 	"github.com/coyove/iis/cmd/ch/mv"
// )
//
// func (m *Manager) IsVote(u *mv.User, aid string) int {
// 	id := makeVoteID(u.ID, aid)
// 	a, _ := m.GetArticle(id)
// 	if a == nil {
// 		return 0
// 	}
// 	v, _ := strconv.Atoi(a.Extras["vote"])
// 	return v
// }
//
// func (m *Manager) Vote(u *mv.User, aid string, vote int) error {
// 	id := makeVoteID(u.ID, aid)
//
// 	m.db.Lock(id)
// 	defer m.db.Unlock(id)
//
// 	a, _ := m.GetArticle(id)
// 	if a != nil {
// 		return fmt.Errorf("vote/already-voted")
// 	}
//
// 	return m.db.Set(id, &mv.Article{
// 		ID:         id,
// 		Cmd:        mv.CmdVote,
// 		Extras:     map[string]string{"vote": strconv.Itoa(vote)},
// 		CreateTime: time.Now(),
// 	})
// }
