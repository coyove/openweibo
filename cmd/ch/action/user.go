package action

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"time"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/ident"
	mv "github.com/coyove/iis/cmd/ch/model"
	"github.com/gin-gonic/gin"
)

func checkCaptcha(g *gin.Context) string {
	var (
		answer            = mv.SoftTrunc(g.PostForm("answer"), 6)
		uuid              = mv.SoftTrunc(g.PostForm("uuid"), 32)
		tokenbuf, tokenok = ident.ParseToken(g, uuid)
		challengePassed   bool
	)

	if !g.GetBool("ip-ok") {
		return fmt.Sprintf("guard/cooling-down/%.1fs", float64(config.Cfg.Cooldown)-g.GetFloat64("ip-ok-remain"))
	}

	if len(answer) == 4 {
		challengePassed = true
		for i := range answer {
			a := answer[i]
			if a >= 'A' && a <= 'Z' {
				a = a - 'A' + 'a'
			}

			if a != "0123456789acefhijklmnpqrtuvwxyz"[tokenbuf[i]%31] &&
				a != "oiz3asg7b9acefhijklmnpqrtuvwxyz"[tokenbuf[i]%31] {
				challengePassed = false
				break
			}
		}
	}

	if !challengePassed {
		log.Println(g.MustGet("ip"), "challenge failed")
		return "guard/failed-captcha"
	}

	if !tokenok {
		return "guard/token-expired"
	}

	return ""
}

func User(g *gin.Context) {
	var (
		ip        = hashIP(g)
		username  = sanUsername(g.PostForm("username"))
		email     = mv.SoftTrunc(g.PostForm("email"), 64)
		passowrd  = mv.SoftTrunc(g.PostForm("password"), 32)
		passowrd2 = mv.SoftTrunc(g.PostForm("password2"), 32)
		redir     = func(a, b string) {
			q := EncodeQuery(a, b, "username", username, "email", email)
			g.Redirect(302, "/user"+q)
		}
		u *mv.User
	)

	if g.PostForm("method") == "logout" {
		g.SetCookie("id", "", 365*86400, "", "", false, false)
		g.Redirect(302, "/user")
		return
	}

	if len(username) < 3 {
		redir("error", "id/too-short")
		return
	}

	m.LockUserID(username)
	defer m.UnlockUserID(username)

	pwdHash := hmac.New(sha256.New, config.Cfg.KeyBytes)
	pwdHash.Write([]byte(passowrd))

	switch g.PostForm("method") {
	case "signup":
		if ret := checkCaptcha(g); ret != "" {
			redir("error", ret)
			return
		}

		if len(passowrd) == 0 || passowrd != passowrd2 {
			redir("error", "password/invalid-too-short")
			return
		}

		if u, err := m.GetUser(username); err == nil && u.ID == username {
			redir("error", "id/already-existed")
			return
		}

		u = &mv.User{
			ID:           username,
			Session:      strconv.FormatInt(time.Now().Unix(), 10) + strconv.FormatInt(rand.Int63(), 10),
			Email:        email,
			PasswordHash: pwdHash.Sum(nil),
			SignupIP:     ip,
			Signup:       time.Now(),
		}
	case "login":
		if ret := checkToken(g); ret != "" {
			redir("error", ret)
			return
		}
		u, _ = m.GetUser(username)
		if u == nil {
			redir("error", "guard/id-not-existed")
			return
		}

		if !bytes.Equal(u.PasswordHash, pwdHash.Sum(nil)) {
			redir("error", "internal/error")
			return
		}
		u.Session = strconv.FormatInt(time.Now().Unix(), 10) + strconv.FormatInt(rand.Int63(), 10)
		u.Login = time.Now()
		u.LoginIP = ip
	}

	if err := m.SetUser(u); err != nil {
		log.Println(u, err)
		redir("error", "internal/error")
		return
	}

	g.SetCookie("id", mv.MakeUserToken(u), 365*86400, "", "", false, false)
	g.Redirect(302, "/cat")
}
