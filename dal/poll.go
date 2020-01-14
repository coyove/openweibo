package dal

//
// import (
// 	"fmt"
// 	"strconv"
// 	"time"
//
// 	"github.com/coyove/iis/model"
// )
//
// func (m *Manager) IsVote(u *model.User, aid string) int {
// 	id := makeVoteID(u.ID, aid)
// 	a, _ := m.GetArticle(id)
// 	if a == nil {
// 		return 0
// 	}
// 	v, _ := strconv.Atoi(a.Extras["vote"])
// 	return v
// }
//
// func (m *Manager) Vote(u *model.User, aid string, vote int) error {
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
// 	return m.db.Set(id, &model.Article{
// 		ID:         id,
// 		Cmd:        model.CmdVote,
// 		Extras:     map[string]string{"vote": strconv.Itoa(vote)},
// 		CreateTime: time.Now(),
// 	})
// }
