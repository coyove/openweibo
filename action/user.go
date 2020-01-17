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

	"github.com/coyove/iis/common"
	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/ik"
	"github.com/coyove/iis/middleware"
	"github.com/coyove/iis/model"
	"github.com/gin-gonic/gin"
)

func Signup(g *gin.Context) {
	var (
		ip       = hashIP(g)
		username = sanUsername(g.PostForm("username"))
		email    = common.SoftTrunc(g.PostForm("email"), 64)
		password = common.SoftTrunc(g.PostForm("password"), 32)
		redir    = func(a, b string, ext ...string) {
			q := common.EncodeQuery(append([]string{a, b, "username", username, "email", email, "password", ik.MakeTempToken(password)}, ext...)...)
			g.Redirect(302, "/user"+q)
		}
	)

	if len(username) < 3 || len(password) < 3 {
		redir("error", "internal/error")
		return
	}

	common.LockKey(username)
	defer common.UnlockKey(username)

	pwdHash := hmac.New(sha256.New, common.Cfg.KeyBytes)
	pwdHash.Write([]byte(password))

	if ret := checkCaptcha(g); ret != "" {
		redir("error", ret)
		return
	}

	if u, err := dal.GetUser(username); err == nil && u.ID == username {
		redir("error", "id/already-existed")
		return
	}

	switch username := strings.ToLower(username); {
	case strings.HasPrefix(username, "master"), strings.HasPrefix(username, "admin"):
		redir("error", "id/already-existed")
		return
	case strings.HasPrefix(username, strings.ToLower(common.Cfg.AdminName)):
		if admin, _ := dal.GetUser(common.Cfg.AdminName); admin != nil {
			redir("error", "id/already-existed")
			return
		}
	}

	u := &model.User{
		ID:           username,
		Session:      genSession(),
		Email:        email,
		PasswordHash: pwdHash.Sum(nil),
		DataIP:       "{" + ip + "}",
		TSignup:      uint32(time.Now().Unix()),
		TLogin:       uint32(time.Now().Unix()),
	}

	if err := dal.SetUser(u); err != nil {
		log.Println(u, err)
		redir("error", "internal/error")
		return
	}

	g.SetCookie("id", ik.MakeUserToken(u), 365*86400, "", "", false, false)
	redir("error", "ok")
}

func APILogin(g *gin.Context) {
	username := sanUsername(g.PostForm("username"))
	password := common.SoftTrunc(g.PostForm("password"), 32)

	if len(username) < 3 {
		g.String(200, "id/too-short")
		return
	}

	common.LockKey(username)
	defer common.UnlockKey(username)

	pwdHash := hmac.New(sha256.New, common.Cfg.KeyBytes)
	pwdHash.Write([]byte(password))

	if ret := checkIP(g); ret != "" {
		g.String(200, ret)
		return
	}

	u, _ := dal.GetUser(username)
	if u == nil {
		g.String(200, "guard/id-not-existed")
		return
	}

	if !bytes.Equal(u.PasswordHash, pwdHash.Sum(nil)) {
		g.String(200, "internal/error")
		return
	}

	u.Session = genSession()
	u.TLogin = uint32(time.Now().Unix())
	if ips := append(strings.Split(u.DataIP, ","), hashIP(g)); len(ips) > 5 {
		u.DataIP = strings.Join(append(ips[:1], ips[len(ips)-4:]...), ",")
	} else {
		u.DataIP = strings.Join(ips, ",")
	}

	if err := dal.SetUser(u); err != nil {
		log.Println(u, err)
		g.String(200, "internal/error")
		return
	}

	g.SetCookie("id", ik.MakeUserToken(u), 365*86400, "", "", false, false)
	g.String(200, "ok")
}

func APIUserKimochi(g *gin.Context) {
	user, _ := g.Get("user")
	u, _ := user.(*model.User)

	if u == nil {
		g.String(200, "internal/error")
		return
	}

	if err := dal.UpdateUser(u.ID, func(u *model.User) error {
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
	uids := common.SearchUsers(g.PostForm("id"), 10)
	for i := range uids {
		uids[i] = "@" + uids[i]
	}
	tags := common.SearchTags(g.PostForm("id"), 10)
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
	p.UUID, p.Challenge = ik.MakeToken(g)
	g.JSON(200, p)
}

func APILike(g *gin.Context) {
	var (
		redir = func(b string) { g.String(200, b) }
		u, _  = dal.GetUserByToken(g.PostForm("api2_uid"))
	)

	if u == nil {
		redir("internal/error")
		return
	}

	if ret := checkIP(g); ret != "" {
		redir(ret)
		return
	}

	common.LockKey(u.ID)
	defer common.UnlockKey(u.ID)

	to := g.PostForm("to")
	if to == "" {
		redir("internal/error")
		return
	}

	err := dal.LikeArticle_unlock(u.ID, to, g.PostForm("like") != "")
	if err != nil {
		redir(err.Error())
	} else {
		redir("ok")
	}
}

func APILogout(g *gin.Context) {
	u := dal.GetUserByContext(g)
	if u != nil {
		dal.UpdateUser(u.ID, func(u *model.User) error {
			u.Session = genSession()
			return nil
		})
		u = &model.User{}
		g.SetCookie("id", ik.MakeUserToken(u), 365*86400, "", "", false, false)
	}
	g.Status(200)
}

func APIFollowBlock(g *gin.Context) {
	u, to := dal.GetUserByContext(g), g.PostForm("to")
	if u == nil || to == "" || u.ID == to {
		g.String(200, "internal/error")
		return
	}

	if ret := checkIP(g); ret != "" {
		g.String(200, ret)
		return
	}

	common.LockKey(u.ID)
	defer common.UnlockKey(u.ID)

	var err error
	if g.PostForm("method") == "follow" {
		err = dal.FollowUser_unlock(u.ID, to, g.PostForm("follow") != "")
	} else {
		err = dal.BlockUser_unlock(u.ID, to, g.PostForm("block") != "")
	}

	if err != nil {
		g.String(200, err.Error())
	} else {
		g.String(200, "ok")
	}
	return
}

func APIFollowBlockSearch(g *gin.Context) {
	u := dal.GetUserByContext(g)
	if u == nil {
		g.String(200, "/user")
		return
	}

	q := g.PostForm("q")
	if strings.HasPrefix(q, "#") {
		g.String(200, "/tag/"+q[1:])
		return
	}

	id := dal.MakeID(g.PostForm("method"), u.ID, q)
	if a, _ := dal.GetArticle(id); a != nil {
		g.String(200, "/user")
		return
	}

	if _, err := dal.GetUser(q); err != nil {
		if res := common.SearchUsers(q, 1); len(res) > 0 {
			q = res[0]
		} else {
			q = ""
		}
	}
	g.String(200, "/t/"+q)
}

func APIUpdateUserSettings(g *gin.Context) {
	u := dal.GetUserByContext(g)
	if u == nil {
		g.String(200, "internal/error")
		return
	}

	update1 := func(cb func(u *model.User)) {
		if err := dal.UpdateUser(u.ID, func(u *model.User) error {
			cb(u)
			return nil
		}); err != nil {
			g.String(200, "internal/error")
		} else {
			g.String(200, "ok")
		}
	}

	update2 := func(cb func(u *model.UserSettings)) {
		if err := dal.UpdateUserSettings(u.ID, func(u *model.UserSettings) error {
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
		update1(func(u *model.User) { u.Email = common.SoftTrunc(g.PostForm("email"), 256) })
	case g.PostForm("set-autonsfw") != "":
		update2(func(u *model.UserSettings) { u.AutoNSFW = g.PostForm("autonsfw") != "" })
	case g.PostForm("set-foldimg") != "":
		update2(func(u *model.UserSettings) { u.FoldImages = g.PostForm("foldimg") != "" })
	case g.PostForm("set-description") != "":
		update2(func(u *model.UserSettings) { u.Description = common.SoftTrunc(g.PostForm("description"), 512) })
	case g.PostForm("set-custom-name") != "":
		name := g.PostForm("custom-name")
		if name := strings.ToLower(name); strings.Contains(name, "admin") && !u.IsAdmin() {
			name = strings.Replace(name, "admin", "nimda", -1)
		}
		name = common.SoftTruncDisplayWidth(name, 16)
		update1(func(u *model.User) {
			u.CustomName = name
			g.Writer.Header().Add("X-Result", url.PathEscape(middleware.RenderTemplateString("display_name.html", u)))
			g.Writer.Header().Add("X-Custom-Name", url.PathEscape(name))
		})
	case g.PostForm("set-avatar") != "":
		_, err := writeAvatar(u, g.PostForm("avatar"))
		if err != nil {
			g.String(200, err.Error())
			return
		}
		update1(func(u *model.User) { u.Avatar++ })
	}
}

func APIUpdateUserPassword(g *gin.Context) {
	u := dal.GetUserByContext(g)
	if u == nil {
		g.String(200, "internal/error")
		return
	}
	if res := checkIP(g); res != "" {
		g.String(200, res)
		return
	}
	if err := dal.UpdateUser(u.ID, func(u *model.User) error {
		oldPassword := common.SoftTrunc(g.PostForm("old-password"), 32)
		newPassword := common.SoftTrunc(g.PostForm("new-password"), 32)

		pwdHash := hmac.New(sha256.New, common.Cfg.KeyBytes)
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
