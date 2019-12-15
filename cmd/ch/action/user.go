package action

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"log"
	"strings"
	"time"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/ident"
	"github.com/coyove/iis/cmd/ch/mv"
	"github.com/gin-gonic/gin"
)

func User(g *gin.Context) {
	var (
		ip        = hashIP(g)
		username  = sanUsername(g.PostForm("username"))
		email     = mv.SoftTrunc(g.PostForm("email"), 64)
		password  = mv.SoftTrunc(g.PostForm("password"), 32)
		password2 = mv.SoftTrunc(g.PostForm("password2"), 32)
		mth       = g.PostForm("method")
		redir     = func(a, b string, ext ...string) {
			if mth == "login" && a == "error" {
				a = "login-error"
			}

			q := EncodeQuery(append([]string{a, b, "username", username, "email", email, "password", ident.MakeTempToken(password)}, ext...)...)

			switch mth {
			case "login", "follow", "block":
				u := g.Request.Referer()
				if idx := strings.Index(u, "?"); idx > -1 {
					u = u[:idx]
				}
				g.Redirect(302, u+q)
			default:
				g.Redirect(302, "/user"+q)
			}
		}
		u *mv.User
	)

	if g.PostForm("cancel") != "" || g.PostForm("refresh") != "" {
		redir("", "")
		return
	}

	if len(username) < 3 {
		redir("error", "id/too-short")
		return
	}

	m.LockUserID(username)
	defer m.UnlockUserID(username)

	pwdHash := hmac.New(sha256.New, config.Cfg.KeyBytes)
	pwdHash.Write([]byte(password))

	switch mth {
	case "signup":
		if ret := checkCaptcha(g); ret != "" {
			redir("error", ret)
			return
		}

		if len(password) == 0 || password != password2 {
			redir("error", "password/invalid-too-short")
			return
		}

		if u, err := m.GetUser(username); err == nil && u.ID == username {
			redir("error", "id/already-existed")
			return
		}

		if username == "master" {
			redir("error", "id/already-existed")
			return
		}

		u = &mv.User{
			ID:           username,
			Session:      genSession(),
			Email:        email,
			PasswordHash: pwdHash.Sum(nil),
			SignupIP:     ip,
			Signup:       time.Now(),
			LoginIP:      ip,
			Login:        time.Now(),
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
		u.Session = genSession()
		u.Login = time.Now()
		u.LoginIP = ip
	case "logout":
		u = &mv.User{}
		g.SetCookie("id", mv.MakeUserToken(u), 365*86400, "", "", false, false)
		g.Redirect(302, "/")
		return
	case "update-info":
		if ret := checkToken(g); ret != "" {
			redir("error", ret)
			return
		}
		user, _ := g.Get("user")
		if user == nil {
			redir("error", "guard/id-not-existed")
			return
		}
		u = user.(*mv.User)
		u.Email = email
		u.Avatar = mv.SoftTrunc(g.PostForm("avatar"), 256)
		u.NoReplyInTimeline = g.PostForm("nrit") != ""
		u.NoPostInMaster = g.PostForm("npim") != ""
		u.AutoNSFW = g.PostForm("autonsfw") != ""
	case "ban-user":
		user, _ := g.Get("user")
		if user == nil {
			redir("error", "internal/error")
			return
		}

		if !user.(*mv.User).IsMod() {
			g.Redirect(302, "/")
			return
		}

		if err := m.UpdateUser_unlock(username, func(u *mv.User) error {
			u.Banned = g.PostForm("ban") != ""
			return nil
		}); err != nil {
			redir("error", err.Error())
		} else {
			g.Redirect(302, "/t/"+username)
		}
		return
	case "follow", "block":
		user, _ := g.Get("user")
		to := g.PostForm("to")

		if user == nil || to == "" || user.(*mv.User).ID == to {
			redir("error", "internal/error")
			return
		}

		if g.PostForm("search") != "" {
			if _, err := m.GetUser(to); err != nil {
				to = mv.SearchUser(to)
			}

			if to == "" {
				redir("error", "user/not-found")
			} else {
				redir("n", "u/"+user.(*mv.User).ID+"/"+mth+"/"+to)
			}
			return
		}

		if ret := checkToken(g); ret != "" {
			redir("error", ret, "n", "u/"+user.(*mv.User).ID+"/"+mth+"/"+to)
			return
		}

		var err error
		if mth == "block" {
			err = m.BlockUser_unlock(user.(*mv.User).ID, to, g.PostForm(mth) != "")
		} else {
			err = m.FollowUser_unlock(user.(*mv.User).ID, to, g.PostForm(mth) != "")
		}

		if err != nil {
			redir("error", err.Error())
		} else {
			redir("error", "ok", "n", "u/"+user.(*mv.User).ID+"/"+mth+"/"+to)
		}
		return
	}

	if err := m.SetUser(u); err != nil {
		log.Println(u, err)
		redir("error", "internal/error")
		return
	}

	g.SetCookie("id", mv.MakeUserToken(u), 365*86400, "", "", false, false)

	if mth == "signup" && username != g.PostForm("username") {
		redir("error", "ok/username-changed")
	} else {
		redir("error", "ok")
	}
}

func UserFollowers(g *gin.Context) {
	redir := func(a ...string) {
		g.Redirect(302, "/user/followers"+EncodeQuery(a...))
	}

	user, _ := g.Get("user")
	u, _ := user.(*mv.User)
	to := g.PostForm("to")

	if u == nil || to == "" || u.ID == to {
		redir("error", "internal/error")
		return
	}

	m.LockUserID(u.ID)
	defer m.UnlockUserID(u.ID)

	if g.PostForm("search") != "" {
		if _, err := m.GetUser(to); err != nil {
			to = mv.SearchUser(to)
		}

		if to == "" {
			redir("error", "user/not-found")
		} else {
			redir("n", "u/"+u.ID+"/followed/"+to)
		}
		return
	}

	if ret := checkToken(g); ret != "" {
		redir("error", ret, "n", "u/"+u.ID+"/followed/"+to)
		return
	}

	err := m.BlockUser_unlock(u.ID, to, true)
	if err != nil {
		redir("error", err.Error())
	} else {
		redir("error", "ok", "n", "u/"+u.ID+"/followed/"+to)
	}
}
