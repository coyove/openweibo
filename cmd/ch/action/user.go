package action

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/ident"
	"github.com/coyove/iis/cmd/ch/manager"
	"github.com/coyove/iis/cmd/ch/mv"
	"github.com/coyove/iis/cmd/ch/view"
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
			q := EncodeQuery(append([]string{a, b, "username", username, "email", email, "password", ident.MakeTempToken(password)}, ext...)...)

			switch mth {
			case "login":
				u := g.Request.Referer()
				if idx := strings.Index(u, "?"); idx > -1 {
					u = u[:idx]
				}
				g.Redirect(302, u+q)
			default:
				g.Redirect(302, "/user"+q)
			}
		}
		u = func() *mv.User {
			u, _ := g.Get("user")
			u2, _ := u.(*mv.User)
			return u2
		}()
	)

	if g.PostForm("cancel") != "" || g.PostForm("refresh") != "" {
		redir("", "")
		return
	}

	if len(username) < 3 {
		redir("error", "id/too-short")
		return
	}

	m.Lock(username)
	defer m.Unlock(username)

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

		if username := strings.ToLower(username); username == "master" ||
			strings.HasPrefix(username, "admin") ||
			strings.HasPrefix(username, strings.ToLower(config.Cfg.AdminName)) {
			redir("error", "id/already-existed")
			return
		}

		u = &mv.User{
			ID:           username,
			Session:      genSession(),
			Email:        email,
			PasswordHash: pwdHash.Sum(nil),
			DataIP:       "{" + ip + "}",
			TSignup:      uint32(time.Now().Unix()),
			TLogin:       uint32(time.Now().Unix()),
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
		u.TLogin = uint32(time.Now().Unix())
		if ips := append(strings.Split(u.DataIP, ","), ip); len(ips) > 5 {
			u.DataIP = strings.Join(append(ips[:1], ips[len(ips)-4:]...), ",")
		} else {
			u.DataIP = strings.Join(ips, ",")
		}
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

func APIUserKimochi(g *gin.Context) {
	user, _ := g.Get("user")
	u, _ := user.(*mv.User)

	if u == nil {
		g.String(200, "internal/error")
		return
	}

	if err := m.UpdateUser(u.ID, func(u *mv.User) error {
		k, _ := strconv.Atoi(g.PostForm("k"))
		if k < 0 || k > 44 {
			k = 25
		}
		u.Kimochi = byte(k)
		return nil
	}); err != nil {
		g.String(200, "internal/error")
		return
	}
	g.String(200, "ok")
}

func APISearch(g *gin.Context) {
	uids := mv.SearchUsers(g.PostForm("id"), 10)
	for i := range uids {
		uids[i] = "@" + uids[i]
	}
	tags := mv.SearchTags(g.PostForm("id"), 10)
	for _, t := range tags {
		uids = append(uids, "#"+t)
	}
	g.JSON(200, uids)
}

func APINewCaptcha(g *gin.Context) {
	var p struct {
		UUID      string
		Challenge string
	}
	p.UUID, p.Challenge = ident.MakeToken(g)
	g.JSON(200, p)
}

func APILike(g *gin.Context) {
	var (
		redir = func(b string) { g.String(200, b) }
		u, _  = m.GetUserByToken(g.PostForm("api2_uid"))
	)

	if u == nil {
		redir("internal/error")
		return
	}

	if ret := checkIP(g); ret != "" {
		redir(ret)
		return
	}

	m.Lock(u.ID)
	defer m.Unlock(u.ID)

	to := g.PostForm("to")
	if to == "" {
		redir("internal/error")
		return
	}

	err := m.LikeArticle_unlock(u.ID, to, g.PostForm("like") != "")
	if err != nil {
		redir(err.Error())
	} else {
		redir("ok")
	}
}

func APILogout(g *gin.Context) {
	u := m.GetUserByContext(g)
	if u != nil {
		m.UpdateUser(u.ID, func(u *mv.User) error {
			u.Session = genSession()
			return nil
		})
		u = &mv.User{}
		g.SetCookie("id", mv.MakeUserToken(u), 365*86400, "", "", false, false)
	}
	g.Status(200)
}

func APIFollowBlock(g *gin.Context) {
	u, to := m.GetUserByContext(g), g.PostForm("to")
	if u == nil || to == "" || u.ID == to {
		g.String(200, "internal/error")
		return
	}

	if ret := checkIP(g); ret != "" {
		g.String(200, ret)
		return
	}

	m.Lock(u.ID)
	defer m.Unlock(u.ID)

	var err error
	if g.PostForm("method") == "follow" {
		err = m.FollowUser_unlock(u.ID, to, g.PostForm("follow") != "")
	} else {
		err = m.BlockUser_unlock(u.ID, to, g.PostForm("block") != "")
	}

	if err != nil {
		g.String(200, err.Error())
	} else {
		g.String(200, "ok")
	}
	return
}

func APIFollowBlockSearch(g *gin.Context) {
	u := m.GetUserByContext(g)
	if u == nil {
		g.String(200, "/user")
		return
	}

	q := g.PostForm("q")
	if strings.HasPrefix(q, "#") {
		g.String(200, "/tag/"+q[1:])
		return
	}

	id := manager.MakeID(g.PostForm("method"), u.ID, q)
	if a, _ := m.GetArticle(id); a != nil {
		g.String(200, "/user")
		return
	}

	if _, err := m.GetUser(q); err != nil {
		if res := mv.SearchUsers(q, 1); len(res) > 0 {
			q = res[0]
		} else {
			q = ""
		}
	}
	g.String(200, "/t/"+q)
}

func APIUpdateUserSettings(g *gin.Context) {
	u := m.GetUserByContext(g)
	if u == nil {
		g.String(200, "internal/error")
		return
	}

	update1 := func(cb func(u *mv.User)) {
		if err := m.UpdateUser(u.ID, func(u *mv.User) error {
			cb(u)
			return nil
		}); err != nil {
			g.String(200, "internal/error")
		} else {
			g.String(200, "ok")
		}
	}

	update2 := func(cb func(u *mv.UserSettings)) {
		if err := m.UpdateUserSettings(u.ID, func(u *mv.UserSettings) error {
			cb(u)
			return nil
		}); err != nil {
			g.String(200, "internal/error")
		} else {
			g.String(200, "ok")
		}
	}

	switch {
	case g.PostForm("set-email") != "":
		update1(func(u *mv.User) { u.Email = mv.SoftTrunc(g.PostForm("email"), 256) })
	case g.PostForm("set-autonsfw") != "":
		update2(func(u *mv.UserSettings) { u.AutoNSFW = g.PostForm("autonsfw") != "" })
	case g.PostForm("set-foldimg") != "":
		update2(func(u *mv.UserSettings) { u.FoldImages = g.PostForm("foldimg") != "" })
	case g.PostForm("set-description") != "":
		update2(func(u *mv.UserSettings) { u.Description = mv.SoftTrunc(g.PostForm("description"), 512) })
	case g.PostForm("set-custom-name") != "":
		name := g.PostForm("custom-name")
		if name := strings.ToLower(name); strings.Contains(name, "admin") && !u.IsAdmin() {
			name = strings.Replace(name, "admin", "nimda", -1)
		}
		name = mv.SoftTruncDisplayWidth(name, 16)
		update1(func(u *mv.User) {
			u.CustomName = name
			g.Writer.Header().Add("X-Result", url.PathEscape(view.RenderTemplateString("display_name.html", u)))
			g.Writer.Header().Add("X-Custom-Name", url.PathEscape(name))
		})
	case g.PostForm("set-avatar") != "":
		_, err := writeAvatar(u, g.PostForm("avatar"))
		if err != nil {
			g.String(200, err.Error())
			return
		}
		update1(func(u *mv.User) { u.Avatar++ })
	}
}

func APIUpdateUserPassword(g *gin.Context) {
	u := m.GetUserByContext(g)
	if u == nil {
		g.String(200, "internal/error")
		return
	}
	if res := checkIP(g); res != "" {
		g.String(200, res)
		return
	}
	if err := m.UpdateUser(u.ID, func(u *mv.User) error {
		oldPassword := mv.SoftTrunc(g.PostForm("old-password"), 32)
		newPassword := mv.SoftTrunc(g.PostForm("new-password"), 32)

		pwdHash := hmac.New(sha256.New, config.Cfg.KeyBytes)
		pwdHash.Write([]byte(oldPassword))
		if len(newPassword) < 3 || !bytes.Equal(u.PasswordHash, pwdHash.Sum(nil)) {
			return fmt.Errorf("password/invalid-too-short")
		}

		pwdHash.Reset()
		pwdHash.Write([]byte(newPassword))
		u.PasswordHash = pwdHash.Sum(nil)
		return nil
	}); err != nil {
		g.String(200, err.Error())
		return
	}
	g.String(200, "ok")
}
