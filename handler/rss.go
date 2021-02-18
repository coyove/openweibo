package handler

import (
	"time"

	"github.com/coyove/iis/common"
	"github.com/gin-gonic/gin"
)

func writeRSS(g *gin.Context, pl ArticlesTimelineView) {
	const host = "https://fweibo.com"

	g.Writer.Header().Add("Content-Type", "application/atom+xml;charset=UTF-8")
	g.Writer.WriteString(`<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
<title>Fweibo</title>

<link rel="alternate" type="text/html" href="` + host + `" />
<link rel="self" type="application/atom+xml" href="` + host + g.Request.RequestURI + `" />
<id>` + host + `</id>

<updated>` + time.Now().UTC().Format("2006-01-02T15:04:05Z") + `</updated>`)

	for _, p := range pl.Articles {
		g.Writer.WriteString(`
    <entry>
        <title>` + common.AbbrText(p.Content, 20) + `</title>
        <link rel="alternate" type="text/html" href="` + host + p.Link + `" />
        <published>` + p.CreateTime.Format("2006-01-02T15:04:05Z") + `</published>
        <author>
            <name>` + p.Author.DisplayName() + `</name>
            <uri>` + host + "/t/" + p.Author.ID + `</uri>
        </author>
        <content type="html"><![CDATA[` + string(p.ContentHTML) + `]]></content>
    </entry>
        `)
	}

	g.Writer.WriteString("</feed>")
}
