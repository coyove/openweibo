package view

import (
	"math"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/gin-gonic/gin"
)

func Error(code int, msg string, g *gin.Context) {
	g.HTML(code, "error.html", struct {
		Tags    []string
		Message string
	}{config.Cfg.Tags, msg})
}

func intmin(a, b int) int {
	return int(math.Min(float64(a), float64(b)))
}

func intdivceil(a, b int) int {
	return int(math.Ceil(float64(a) / float64(b)))
}
