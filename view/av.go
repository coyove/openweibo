package view

import (
	"encoding/base64"
	"html/template"
	"strings"
	"time"

	"github.com/coyove/iis/common"
	"github.com/coyove/iis/ik"
	"github.com/coyove/iis/model"
)

type ArticleView struct {
	ID          string
	IDDOM       string
	Parent      *ArticleView
	Author      *model.User
	You         *model.User
	Cmd         string
	Replies     int
	Likes       int
	Locked      bool
	Liked       bool
	NSFW        bool
	NoAvatar    bool
	Blank       bool
	Content     string
	ContentHTML template.HTML
	Media       string
	MediaType   string
	CreateTime  time.Time
}

const (
	_ uint64 = 1 << iota
	_NoMoreParent
	_ShowAvatar
	_Blank
)

func NewTopArticleView(a *model.Article, you *model.User) (av ArticleView) {
	av.from(a, 0, you)
	return
}

func NewReplyArticleView(a *model.Article, you *model.User) (av ArticleView) {
	av.from(a, _NoMoreParent|_ShowAvatar, you)
	return
}

func (a *ArticleView) from(a2 *model.Article, opt uint64, u *model.User) *ArticleView {
	if a2 == nil {
		return a
	}

	a.ID = a2.ID
	a.Replies = a2.Replies
	a.Likes = int(a2.Likes)
	a.Locked = a2.Locked
	a.NSFW = a2.NSFW
	a.Blank = opt&_Blank > 0
	a.Cmd = string(a2.Cmd)
	a.CreateTime = a2.CreateTime
	a.Author, _ = m.GetUser(a2.Author)
	if a.Author == nil {
		a.Author = &model.User{
			ID: a2.Author + "?",
		}
	}
	a.You = u
	if a.You == nil {
		a.You = &model.User{}
	} else {
		a.Liked = m.IsLiking(u.ID, a2.ID)
	}

	if p := strings.SplitN(a2.Media, ":", 2); len(p) == 2 {
		a.MediaType, a.Media = p[0], p[1]
		switch a.MediaType {
		case "IMG":
			if strings.HasPrefix(a.Media, "LOCAL:") {
				a.Media = "/i/" + a.Media[6:] + ".jpg"
			} else {
				a.Media = "/img/" + base64.StdEncoding.EncodeToString([]byte(a.Media)) + ".jpg"
			}
		}
	}

	if img := common.ExtractFirstImage(a2.Content); img != "" && a2.Media == "" {
		a.MediaType, a.Media = "IMG", img
	}

	a.Content = a2.Content
	a.ContentHTML = a2.ContentHTML()

	if a2.Parent != "" {
		a.Parent = &ArticleView{}
		if opt&_NoMoreParent == 0 {
			p, _ := m.GetArticle(a2.Parent)
			a.Parent.from(p, opt|_NoMoreParent, u)
		}
	}

	a.NoAvatar = opt&_NoMoreParent > 0
	if opt&_ShowAvatar > 0 {
		a.NoAvatar = false
	}

	switch a2.Cmd {
	case model.CmdReply, model.CmdMention:
		p, _ := m.GetArticle(a2.Extras["article_id"])
		if p == nil {
			return a
		}

		a.from(p, opt, u)
		a.Cmd = string(a2.Cmd)
	case model.CmdILike:
		p, _ := m.GetArticle(a2.Extras["article_id"])

		if p == nil {
			return a
		}

		dummy := &model.Article{
			ID:         ik.NewGeneralID().String(),
			CreateTime: p.CreateTime,
			Cmd:        model.CmdILike,
			Author:     a2.Extras["from"],
			Parent:     p.ID,
		}
		a.from(dummy, opt, u)
	}

	return a
}

func fromMultiple(a *[]ArticleView, a2 []*model.Article, opt uint64, u *model.User) {
	*a = make([]ArticleView, len(a2))
	for i, v := range a2 {
		(*a)[i].from(v, opt, u)
	}
}
