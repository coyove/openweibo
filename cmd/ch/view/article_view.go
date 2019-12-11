package view

import (
	"html/template"

	"github.com/coyove/iis/cmd/ch/mv"
)

type ArticleView struct {
	ID       string
	Timeline string
	Parent   *ArticleView
	Image    string
	Index    int
	SubIndex string
	Replies  int
	Forwards int
	Upvotes  int
	//Views       uint32
	Locked      bool
	Highlighted bool
	Title       string
	Content     string
	ContentHTML template.HTML
	Author      string
	IP          string
	Category    string
	CreateTime  string
	ReplyTime   string
}

const (
	_ uint64 = 1 << iota
	_abstract
	_nomoreparent
	_richtime
)

func (a *ArticleView) from(a2 *mv.Article, opt uint64) *ArticleView {
	a.ID = a2.ID
	a.Replies = a2.Replies
	a.Locked = a2.Locked
	a.Highlighted = a2.Highlighted
	a.Author = a2.Author
	a.IP = a2.IP
	a.Category = a2.Category
	a.CreateTime = mv.FormatTime(a2.CreateTime, opt&_richtime > 0)

	if opt&_abstract > 0 {
		a.Content = mv.SoftTrunc(a2.Content, 64)
		a.ContentHTML = template.HTML(template.HTMLEscapeString(a.Content))
	} else {
		a.Content = a2.Content
		a.ContentHTML = a2.ContentHTML()
	}

	if a2.Parent != "" && opt&_nomoreparent == 0 {
		p, _ := m.GetArticle(a2.Parent)
		a.Parent = &ArticleView{}
		a.Parent.from(p, opt|_nomoreparent)
	}

	return a
}

func fromMultiple(a *[]ArticleView, a2 []*mv.Article, opt uint64) {
	*a = make([]ArticleView, len(a2))
	for i, v := range a2 {
		(*a)[i].from(v, opt)
	}
}
