package handler

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/coyove/iis/common"
	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/ik"
	"github.com/coyove/iis/middleware"
	"github.com/coyove/iis/model"
	"github.com/gin-gonic/gin"
)

func RPCGetUserInfo(g *gin.Context) {
	if common.Cfg.RPCKey == "" || g.GetHeader("X-Key") != common.Cfg.RPCKey {
		g.Writer.Header().Add("X-Result", "error")
		g.String(200, "invalid/key")
		return
	}

	var u *model.User
	var err error

	switch {
	case g.PostForm("id") != "":
		u, err = dal.GetUserWithSettings(g.PostForm("id"))

		if pwd := g.PostForm("password"); pwd != "" {
			if !bytes.Equal(u.PasswordHash, hashPassword(pwd)) {
				err = fmt.Errorf("invalid/password")
			}
		}
	case g.PostForm("cookie") != "":
		u, err = dal.GetUserByToken(g.PostForm("cookie"))
	}

	if err != nil {
		g.Writer.Header().Add("X-Result", "error")
		g.String(200, err.Error())
		return
	}

	g.Writer.Header().Add("X-Result", "ok")
	g.JSON(200, u)
}

func APISignup(g *gin.Context) {
	var (
		ip       = hashIP(g)
		username = sanUsername(g.PostForm("username"))
		email    = common.SoftTrunc(g.PostForm("email"), 64)
		password = common.SoftTrunc(g.PostForm("password"), 32)
		ott      = g.PostForm("ott")
	)

	if len(username) < 3 || len(password) < 3 {
		g.String(200, "internal/error")
		return
	}

	if ott != "" {
		if !ik.ValidateOTT(g.ClientIP(), ott) {
			g.String(200, "token/invalid-ott")
			return
		}

		if flag, _ := dal.CacheGet(ott); flag == "1" {
			g.String(200, "token/used-ott")
			return
		}
	} else {
		if ret := checkCaptcha(g); ret != "" {
			g.String(200, ret)
			return
		}
	}

	switch username := strings.ToLower(username); {
	case strings.HasPrefix(username, "master"), strings.HasPrefix(username, "admin"):
		g.String(200, "id/already-existed")
		return
	case strings.HasPrefix(username, strings.ToLower(common.Cfg.AdminName)):
		if admin, _ := dal.GetUser(common.Cfg.AdminName); admin != nil {
			g.String(200, "id/already-existed")
			return
		}
	}

	u := &model.User{}
	u.ID = username
	u.Session = genSession()
	u.Email = email
	u.PasswordHash = hashPassword(password)
	u.DataIP = ip
	u.TSignup = uint32(time.Now().Unix())
	u.TLogin = uint32(time.Now().Unix())
	tok := ik.MakeUserToken(u)

	if _, err := dal.DoUpdateUser(&dal.UpdateUserRequest{
		Signup:       true,
		ID:           u.ID,
		Session:      aws.String(u.Session),
		Email:        aws.String(u.Email),
		PasswordHash: &u.PasswordHash,
		DataIP:       aws.String(u.DataIP),
		TSignup:      aws.Uint32(u.TSignup),
		TLogin:       aws.Uint32(u.TLogin),
	}); err != nil {
		g.String(200, err.Error())
		return
	}

	g.SetCookie("id", tok, 365*86400, "", "", false, false)
	g.String(200, "ok")

	if ott != "" {
		dal.CacheSet(ott, "1")
	}
}

func APILogin(g *gin.Context) {
	if ret := checkIP(g); ret != "" {
		g.String(200, ret)
		return
	}

	u, _ := dal.GetUser(sanUsername(g.PostForm("username")))
	if u == nil {
		g.String(200, "id/too-short")
		return
	}

	if !bytes.Equal(u.PasswordHash, hashPassword(g.PostForm("password"))) {
		g.String(200, "internal/error")
		return
	}

	// u.Session = genSession()
	u.TLogin = uint32(time.Now().Unix())

	if ips := append(strings.Split(u.DataIP, ","), hashIP(g)); len(ips) > 3 {
		u.DataIP = strings.Join(ips[len(ips)-3:], ",")
	} else {
		u.DataIP = strings.Join(ips, ",")
	}

	tok := ik.MakeUserToken(u)

	if _, err := dal.DoUpdateUser(&dal.UpdateUserRequest{
		ID:      u.ID,
		Session: aws.String(u.Session),
		DataIP:  aws.String(u.DataIP),
		TLogin:  aws.Uint32(u.TLogin),
	}); err != nil {
		g.String(200, err.Error())
	} else {
		ttl := 0
		if g.PostForm("remember") != "" {
			ttl = 365 * 86400
		}
		g.SetCookie("id", tok, ttl, "", "", false, false)
		g.String(200, "ok")
	}
}

func APIUserKimochi(g *gin.Context) {
	u := dal.GetUserByContext(g)
	if u == nil {
		g.String(200, "user/404")
		return
	}

	k, _ := strconv.Atoi(g.PostForm("k"))
	if k < 0 || k > 44 {
		k = 25
	}

	if _, err := dal.DoUpdateUser(&dal.UpdateUserRequest{ID: u.ID, Kimochi: aws.Uint8(byte(k))}); err != nil {
		g.String(200, "internal/error")
		return
	}
	g.String(200, "ok")
}

func APISearch(g *gin.Context) {
	type p struct {
		ID      string
		Display string
		IsTag   bool
	}
	results := []p{}
	uids, _, _ := model.Search("su", g.PostForm("id"), 0, 10)
	for i := range uids {
		if u, _ := dal.GetUser(uids[i]); u != nil {
			results = append(results, p{Display: u.DisplayName(), ID: uids[i]})
		}
	}
	tags, _, _ := model.Search("st", g.PostForm("id"), 0, 10)
	for _, t := range tags {
		results = append(results, p{Display: "#" + t, ID: t, IsTag: true})
	}
	g.JSON(200, results)
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
	u := dal.GetUserByContext(g)

	if u == nil {
		g.String(200, "user/404")
		return
	}

	if ret := checkIP(g); ret != "" {
		g.String(200, ret)
		return
	}

	to := g.PostForm("to")
	if to == "" {
		g.String(200, "internal/error")
		return
	}

	err := dal.LikeArticle(u, to, g.PostForm("like") != "")
	if err != nil {
		g.String(200, err.Error())
	} else {
		g.String(200, "ok")
	}
}

func APILogout(g *gin.Context) {
	u := dal.GetUserByContext(g)
	if u != nil {
		dal.DoUpdateUser(&dal.UpdateUserRequest{
			ID:      u.ID,
			Session: aws.String(genSession()),
		})
		u = &model.User{}
		g.SetCookie("id", ik.MakeUserToken(u), 365*86400, "", "", false, false)
	}
	g.String(200, "ok")
}

func APIFollowBlock(g *gin.Context) {
	u, to := dal.GetUserByContext(g), g.PostForm("to")
	if u == nil {
		g.String(200, "user/404")
		return
	}

	if to == "" || u.ID == to {
		g.String(200, "internal/error")
		return
	}

	if ret := checkIP(g); ret != "" {
		g.String(200, ret)
		return
	}

	var err error
	switch g.PostForm("method") {
	case "follow":
		err = dal.FollowUser(u.ID, to, g.PostForm("follow") != "")
	case "accept":
		err = dal.AcceptUser(u.ID, to, g.PostForm("accept") != "")
	default:
		if strings.HasPrefix(to, "#") {
			g.String(200, "error/block-tag")
			return
		}
		err = dal.BlockUser(u.ID, to, g.PostForm("block") != "")
	}

	if err != nil {
		g.String(200, err.Error())
	} else {
		g.String(200, "ok")
	}
	return
}

func APIUpdateUserSettings(g *gin.Context) {
	u := dal.GetUserByContext(g)
	if u == nil {
		g.String(200, "user/404")
		return
	}

	switch {
	case g.PostForm("set-email") != "":
		if _, err := dal.DoUpdateUser(&dal.UpdateUserRequest{
			ID:    u.ID,
			Email: aws.String(common.SoftTrunc(g.PostForm("email"), 256)),
		}); err != nil {
			g.String(200, err.Error())
			return
		}
	case g.PostForm("set-autonsfw") != "":
		if _, err := dal.DoUpdateUser(&dal.UpdateUserRequest{
			ID:              u.ID,
			SettingAutoNSFW: aws.Bool(g.PostForm("autonsfw") != ""),
		}); err != nil {
			g.String(200, err.Error())
			return
		}
	case g.PostForm("set-foldimg") != "":
		if _, err := dal.DoUpdateUser(&dal.UpdateUserRequest{
			ID:                u.ID,
			SettingFoldImages: aws.Bool(g.PostForm("foldimg") != ""),
		}); err != nil {
			g.String(200, err.Error())
			return
		}
	case g.PostForm("set-mffm") != "":
		if _, err := dal.DoUpdateUser(&dal.UpdateUserRequest{
			ID:          u.ID,
			SettingMFFM: aws.Bool(g.PostForm("mffm") != ""),
		}); err != nil {
			g.String(200, err.Error())
			return
		}
	case g.PostForm("set-hl") != "":
		if _, err := dal.DoUpdateUser(&dal.UpdateUserRequest{
			ID:        u.ID,
			SettingHL: aws.Bool(g.PostForm("hl") != ""),
		}); err != nil {
			g.String(200, err.Error())
			return
		}
	case g.PostForm("set-slit") != "":
		if _, err := dal.DoUpdateUser(&dal.UpdateUserRequest{
			ID:          u.ID,
			SettingSLIT: aws.Bool(g.PostForm("slit") != ""),
		}); err != nil {
			g.String(200, err.Error())
			return
		}
	case g.PostForm("set-mfcm") != "":
		if _, err := dal.DoUpdateUser(&dal.UpdateUserRequest{
			ID:          u.ID,
			SettingMFCM: aws.Bool(g.PostForm("mfcm") != ""),
		}); err != nil {
			g.String(200, err.Error())
			return
		}
	case g.PostForm("set-description") != "":
		if _, err := dal.DoUpdateUser(&dal.UpdateUserRequest{
			ID:                 u.ID,
			SettingDescription: aws.String(common.SoftTrunc(g.PostForm("description"), 512)),
		}); err != nil {
			g.String(200, err.Error())
			return
		}
	case g.PostForm("set-fw-accept") != "":
		if _, err := dal.DoUpdateUser(&dal.UpdateUserRequest{
			ID:              u.ID,
			SettingFwAccept: aws.Bool(g.PostForm("fw-accept") != ""),
		}); err != nil {
			g.String(200, err.Error())
			return
		}
	case g.PostForm("set-custom-name") != "":
		name := g.PostForm("custom-name")
		if strings.Contains(strings.ToLower(name), "admin") && !u.IsAdmin() {
			name = strings.Replace(name, "admin", "nimda", -1)
		}
		name = common.SoftTruncDisplayWidth(name, 16)
		u2, err := dal.DoUpdateUser(&dal.UpdateUserRequest{ID: u.ID, CustomName: &name})
		if err != nil {
			g.String(200, err.Error())
			return
		}
		g.Writer.Header().Add("X-Result",
			url.PathEscape(middleware.RenderTemplateString("display_name.html", u2)))
		g.Writer.Header().Add("X-Custom-Name", url.PathEscape(name))
		model.IndexUser(&u2, true)
	case g.PostForm("set-avatar") != "":
		_, err := writeAvatar(u, g.PostForm("avatar"))
		if err != nil {
			g.String(200, err.Error())
			return
		}
		if _, err := dal.DoUpdateUser(&dal.UpdateUserRequest{ID: u.ID, Avatar: aws.Uint32(uint32(time.Now().Unix()))}); err != nil {
			g.String(200, err.Error())
			return
		}
	}
	go func() {
		if ips := append(strings.Split(u.DataIP, ","), hashIP(g)); len(ips) > 3 {
			u.DataIP = strings.Join(ips[len(ips)-3:], ",")
		} else {
			u.DataIP = strings.Join(ips, ",")
		}
		dal.DoUpdateUser(&dal.UpdateUserRequest{
			ID:     u.ID,
			DataIP: aws.String(u.DataIP),
		})
	}()
	g.String(200, "ok")
}

func APIUpdateUserPassword(g *gin.Context) {
	u := dal.GetUserByContext(g)
	if u == nil {
		g.String(200, "user/404")
		return
	}
	if res := checkIP(g); res != "" {
		g.String(200, res)
		return
	}

	oldPassword := common.SoftTrunc(g.PostForm("old-password"), 32)
	newPassword := common.SoftTrunc(g.PostForm("new-password"), 32)

	if len(newPassword) < 3 || !bytes.Equal(u.PasswordHash, hashPassword(oldPassword)) {
		g.String(200, "password/invalid-too-short")
		return
	}

	hp := hashPassword(newPassword)
	if _, err := dal.DoUpdateUser(&dal.UpdateUserRequest{ID: u.ID, PasswordHash: &hp}); err != nil {
		g.String(200, err.Error())
		return
	}
	g.String(200, "ok")
}

func APIResetUserPassword(g *gin.Context) {
	u := dal.GetUserByContext(g)
	if u == nil {
		g.String(200, "user/404")
		return
	}
	if res := checkIP(g); res != "" {
		g.String(200, res)
		return
	}

	username := sanUsername(g.PostForm("username"))
	email := common.SoftTrunc(g.PostForm("email"), 64)

	u, err := dal.GetUser(username)
	if err != nil {
		log.Println("Reset password, get user err: %v", err)
		g.String(200, "internal/error")
		return
	}
	if u.Email != email {
		log.Println("Reset password, email not matched: %q, %q", u.Email, email)
		g.String(200, "internal/error")
		return
	}

	newPassword, _ := ioutil.ReadAll(io.LimitReader(rand.Reader, 8))
	hp := hashPassword(hex.EncodeToString(newPassword))
	if _, err := dal.DoUpdateUser(&dal.UpdateUserRequest{ID: u.ID, PasswordHash: &hp}); err != nil {
		g.String(200, err.Error())
		return
	}

	if err := common.SendMail(email,
		"Password Reset",
		fmt.Sprintf("Username: %q, New Password: %v", u.ID, hex.EncodeToString(newPassword))); err != nil {
		log.Println("Reset password, email send failure: %v", err)
		g.String(200, "internal/error")
		return
	}
	g.String(200, "ok")
}

func APIClearInbox(g *gin.Context) {
	u := dal.GetUserByContext(g)
	if u == nil {
		g.String(200, "user/404")
		return
	}
	if err := dal.ClearInbox(u.ID); err != nil {
		g.String(200, err.Error())
		return
	}
	g.String(200, "ok")
}
