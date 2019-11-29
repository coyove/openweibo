package view

import (
	"html/template"
	"strconv"

	mv "github.com/coyove/iis/cmd/ch/model"
)

type ArticleView struct {
	ID        string
	Timeline  string
	Parent    string
	TopParent string
	Image     string
	Index     int
	SubIndex  string
	Replies   int
	//Views       uint32
	Locked      bool
	Highlighted bool
	Saged       bool
	Title       string
	Content     string
	ContentHTML template.HTML
	Author      string
	IP          string
	Category    string
	CreateTime  string
	ReplyTime   string
	ShowReplies struct {
		First  *ArticleView
		Last   *ArticleView
		More   bool
		Hidden bool
	}
}

const (
	_ uint64 = 1 << iota
	_abstract
	_abstracttitle
	_showcontent
	_showsubreplies
	_richtime
)

func (a *ArticleView) from(a2 *mv.Article, opt uint64) *ArticleView {
	a.ID = a2.ID
	a.Timeline = a2.TimelineID
	a.Index = a2.Index()
	if opt>>48 > 0 {
		i := opt >> 48
		opt = opt << 16 >> 16
		a.SubIndex = strconv.FormatUint(i, 10) + "." + strconv.Itoa(a.Index)
	}

	a.Replies = a2.Replies
	a.Locked = a2.Locked
	a.Highlighted = a2.Highlighted
	a.Saged = a2.Saged
	a.Author = a2.Author
	a.IP = a2.IP
	a.Category = a2.Category
	a.CreateTime = mv.FormatTime(a2.CreateTime, opt&_richtime > 0)
	a.ReplyTime = mv.FormatTime(a2.ReplyTime, opt&_richtime > 0)
	a.Parent, a.TopParent = a2.Parent()

	if opt&_abstract > 0 {
		a.Content = mv.SoftTrunc(a2.Content, 64)
		a.Image = a2.Image
		if len(a.Content) == 0 && a.Image != "" {
			a.Content = a.Image
		}
		a.ContentHTML = template.HTML(template.HTMLEscapeString(a.Content))
	} else if opt&_showcontent > 0 {
		a.Content = a2.Content
		a.Image = a2.Image
		a.ContentHTML = a2.ContentHTML()
	}

	if opt&_abstracttitle > 0 {
		a.Title = mv.SoftTrunc(a2.Title, 20)
	} else {
		a.Title = a2.Title
	}

	if opt&_showsubreplies > 0 {
		opt ^= _showsubreplies
		opt |= _abstract
		opt |= uint64(a.Index) << 48

		if a2.Replies == 1 {
			a.ShowReplies.First = (&ArticleView{}).from(m.GetReplies(a2.ID, 1, 2)[0], opt)
			a.ShowReplies.First.ShowReplies.Hidden = true
		} else if a2.Replies > 1 {
			a.ShowReplies.First = (&ArticleView{}).from(m.GetReplies(a2.ID, 1, 2)[0], opt)
			a.ShowReplies.Last = (&ArticleView{}).from(m.GetReplies(a2.ID, a2.Replies, a2.Replies+1)[0], opt)
			a.ShowReplies.First.ShowReplies.Hidden = true
			a.ShowReplies.Last.ShowReplies.Hidden = true
			a.ShowReplies.More = a2.Replies > 2
		}
	}

	return a
}

func fromMultiple(a *[]ArticleView, a2 []*mv.Article, opt uint64) {
	*a = make([]ArticleView, len(a2))
	for i, v := range a2 {
		(*a)[i].from(v, opt)
	}
}
