package view

import (
	"html/template"

	"github.com/coyove/iis/cmd/ch/ident"
	mv "github.com/coyove/iis/cmd/ch/model"
	"github.com/gin-gonic/gin"
)

type ArticleView struct {
	ID          string
	Timeline    string
	Parent      string
	TopParent   string
	Index       uint32
	Replies     uint32
	Views       uint32
	Locked      bool
	Highlighted bool
	Saged       bool
	Announce    bool
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
	_obfs byte = 1 << iota
	_abstract
	_abstracttitle
	_showcontent
	_richtime
)

func (a *ArticleView) from(a2 *mv.Article, opt byte, g *gin.Context) {
	a.Index = uint32(a2.Index)
	a.Replies = uint32(a2.Replies)
	a.Views = uint32(a2.Views)
	a.Locked = a2.Locked
	a.Highlighted = a2.Highlighted
	a.Saged = a2.Saged
	a.Announce = a2.Announce
	a.Title = a2.Title
	a.Author = a2.Author
	a.IP = a2.IP
	a.Category = a2.Category
	a.CreateTime = mv.FormatTime(a2.CreateTime, opt&_richtime > 0)
	a.ReplyTime = mv.FormatTime(a2.ReplyTime, opt&_richtime > 0)

	if opt&_abstract > 0 {
		a.Content = mv.SoftTrunc(a2.Content, 64)
	} else if opt&_showcontent > 0 {
		a.ContentHTML = a2.ContentHTML()
		a.Content = a2.Content
	}

	if opt&_abstracttitle > 0 {
		a.Title = mv.SoftTrunc(a2.Title, 20)
	}

	parent, topparent := ident.ParseID(a2.ID).RIndexParent()

	if opt&_obfs > 0 {
		a.ID = ident.BytesString(g, a2.ID)
		a.Timeline = ident.BytesString(g, a2.Timeline)
		a.Parent = ident.BytesString(g, parent.Marshal())
		a.TopParent = ident.BytesString(g, topparent.Marshal())
	} else {
		a.ID = ident.BytesPlainString(a2.ID)
		a.Timeline = ident.BytesPlainString(a2.Timeline)
		a.Parent = ident.BytesPlainString(parent.Marshal())
		a.TopParent = ident.BytesPlainString(topparent.Marshal())
	}
}

func fromMultiple(a *[]ArticleView, a2 []*mv.Article, opt byte, g *gin.Context) {
	*a = make([]ArticleView, len(a2))
	for i, v := range a2 {
		(*a)[i].from(v, opt, g)
	}
}
