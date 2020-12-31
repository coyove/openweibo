package middleware

import (
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/coyove/iis/model"
	"github.com/gin-gonic/gin"
)

type Error struct {
	From  string
	Value interface{}
	Code  string
}

func ThrowIf(v interface{}, code string) interface{} {
	pc, fn, line, _ := runtime.Caller(1)
	fun, _ := runtime.CallersFrames([]uintptr{pc}).Next()

	if v == nil {
		return nil
	}

	switch v := v.(type) {
	case error:
		if v == nil {
			return v
		}
	case bool:
		if !v {
			return v
		}
	case string:
		if v == "" {
			return v
		}
		code = v
	case int:
		if v == 0 {
			return v
		}
	case *model.User:
		if v != nil {
			return v
		}
		if code == "" {
			code = "user_not_found"
		}
	default:
	}

	panic(&Error{
		From:  fmt.Sprintf("`%s:%s:%d`", filepath.Base(fn), filepath.Base(fun.Function), line),
		Value: v,
		Code:  code,
	})
}

func errorHandling(g *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			var code string
			if err, ok := r.(*Error); ok {
				log.Println(err.From, err.Value)
				code = err.Code
				if x := fmt.Sprint(err.Value); strings.HasPrefix(x, "e:") {
					code = x[2:]
				}
			} else {
				if rs := fmt.Sprint(r); strings.Contains(rs, "broken pipe") {
					log.Println(rs)
				} else {
					log.Println(rs, string(debug.Stack()))
				}
			}
			if code == "" {
				code = "internal_error"
			}

			if !g.Writer.Written() {
				if g.GetBool("error-as-500") {
					g.String(500, code)
				} else {
					g.String(200, code)
				}
			}
			g.Abort()
		}
	}()
	g.Next()
}
