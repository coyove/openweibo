package handler

import (
	"bytes"
	"strconv"
	"strings"
	"time"

	"github.com/coyove/iis/common"
	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/ik"
	"github.com/coyove/iis/model"
	"github.com/gin-gonic/gin"
)

func APISignup(g *gin.Context) {
	var (
		username = sanUsername(g.PostForm("username"))
		email    = common.SoftTrunc(g.PostForm("email"), 64)
		password = common.SoftTrunc(g.PostForm("password"), 32)
	)

	throw(len(username) < 3 || len(password) < 3, "id_too_short")
	throw(checkCaptcha(g), "")

	switch username := strings.ToLower(username); {
	case strings.HasPrefix(username, "master"), strings.HasPrefix(username, "admin"):
		throw(true, "duplicated_id")
	case strings.HasPrefix(username, strings.ToLower(common.Cfg.AdminName)):
		admin, _ := dal.GetUser(common.Cfg.AdminName)
		throw(admin != nil, "duplicated_id")
	}

	session := genSession()
	throw(dal.DoSignUp(username, hashPassword(password), email, session, hashIP(g)), "")

	tok := ik.MakeUserToken(username, session)
	g.SetCookie("id", tok, 365*86400, "", "", false, false)
	okok(g)
}

func APILogin(g *gin.Context) {
	throw(checkIP(g), "")

	u, _ := dal.GetUser(sanUsername(g.PostForm("username")))
	throw(u, "invalid_id_password")
	throw(!bytes.Equal(u.PasswordHash, hashPassword(g.PostForm("password"))), "invalid_id_password")
	throw(common.Err2(dal.DoUpdateUser(u.ID, func(u2 *model.User) {
		u2.DataIP = common.PushIP(u.DataIP, hashIP(g))
		u2.TLogin = uint32(time.Now().Unix())
	})), "")

	ttl := 0
	if g.PostForm("remember") != "" {
		ttl = 365 * 86400
	}
	g.SetCookie("id", ik.MakeUserToken(u.ID, u.Session), ttl, "", "", false, false)
	okok(g)
}

func APIUserKimochi(g *gin.Context) {
	u := throw(dal.GetUserByContext(g), "").(*model.User)
	k, _ := strconv.Atoi(g.PostForm("k"))
	if k < 0 || k > 44 {
		k = 25
	}
	throw(common.Err2(dal.DoUpdateUser(u.ID, "Kimochi", byte(k))), "")
	okok(g)
}

func APISearch(g *gin.Context) {
	type p struct {
		ID      string
		Display string
		IsTag   bool
	}
	results := []p{}
	uids, _ := model.Search("su", g.PostForm("id"), 0, 10)
	for i := range uids {
		if u, _ := dal.GetUser(uids[i].Tag()); u != nil {
			results = append(results, p{Display: u.DisplayName(), ID: uids[i].Tag()})
		}
	}
	tags, _ := model.Search("st", g.PostForm("id"), 0, 10)
	for _, t := range tags {
		results = append(results, p{Display: "#" + t.Tag(), ID: t.Tag(), IsTag: true})
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
	u := throw(dal.GetUserByContext(g), "").(*model.User)
	to := g.PostForm("to")

	throw(checkIP(g), "")
	throw(to == "", "")
	throw(dal.LikeArticle(u, to, g.PostForm("like") != ""), "")
	okok(g)
}

func APILogout(g *gin.Context) {
	u := dal.GetUserByContext(g)
	if u != nil {
		dal.DoUpdateUser(u.ID, "Session", "")
		g.SetCookie("id", ik.MakeUserToken("", ""), 365*86400, "", "", false, false)
	}
	okok(g)
}

func APIFollowBlock(g *gin.Context) {
	u, to := dal.GetUserByContext(g), g.PostForm("to")
	throw(u, "")
	throw(to == "" || u.ID == to, "")
	throw(checkIP(g), "")
	isTag := strings.HasPrefix(to, "#")

	switch g.PostForm("method") {
	case "follow":
		following := g.PostForm("follow") != ""
		if following && !isTag {
			throw(dal.IsBlocking(to, u.ID), "cannot_follow")
		}
		throw(dal.FollowUser(u.ID, to, following), "")
		if !isTag {
			toUser, _ := dal.WeakGetUser(to)
			if toUser != nil && toUser.FollowApply != 0 {
				g.Writer.Header().Add("X-Follow-Apply", "1")
			}
		}
	case "accept":
		throw(isTag, "cannot_accept_tag")
		throw(dal.AcceptUser(u.ID, to, true), "")
		// Given the situation that there may be A LOT applications received by one user
		g.Set("clear-ip-throt", true)
	default:
		throw(isTag, "cannot_block_tag")
		throw(dal.BlockUser(u.ID, to, g.PostForm("block") != ""), "")
	}
	okok(g)
}

func APIUpdateUserSettings(g *gin.Context) {
	u := throw(dal.GetUserByContext(g), "").(*model.User)

	switch {
	case g.PostForm("set-email") != "":
		throw(common.Err2(dal.DoUpdateUser(u.ID, "Email", common.SoftTrunc(g.PostForm("email"), 256))), "")
	case g.PostForm("set-autonsfw") != "":
		throw(common.Err2(dal.DoUpdateUser(u.ID, "ExpandNSFWImages", common.BoolInt(g.PostForm("autonsfw") != ""))), "")
	case g.PostForm("set-foldimg") != "":
		throw(common.Err2(dal.DoUpdateUser(u.ID, "FoldAllImages", common.BoolInt(g.PostForm("foldimg") != ""))), "")
	case g.PostForm("set-hl") != "":
		throw(common.Err2(dal.DoUpdateUser(u.ID, "HideLocation", common.BoolInt(g.PostForm("hl") != ""))), "")
	case g.PostForm("set-hide-likes") != "":
		throw(common.Err2(dal.DoUpdateUser(u.ID, "HideLikes", common.BoolInt(g.PostForm("hide-likes") != ""))), "")
	case g.PostForm("set-mfcm") != "":
		throw(common.Err2(dal.DoUpdateUser(u.ID, "NotifyFollowerActOnly", common.BoolInt(g.PostForm("mfcm") != ""))), "")
	case g.PostForm("set-description") != "":
		throw(common.Err2(dal.DoUpdateUser(u.ID, "Description", common.SoftTrunc(g.PostForm("description"), 512))), "")
	case g.PostForm("set-apisession") != "":
		apiToken := ik.MakeUserToken(u.ID, "api+"+genSession())
		throw(common.Err2(dal.DoUpdateUser(u.ID, "APIToken", apiToken)), "")
		okok(g, apiToken)
		return
	case g.PostForm("set-fw-accept") != "":
		throw(common.Err2(dal.DoUpdateUser(u.ID, func(u *model.User) {
			if g.PostForm("fw-accept") != "" {
				u.FollowApply = uint32(time.Now().Unix())
			} else {
				u.FollowApply = 0
			}
		})), "")
	case g.PostForm("set-custom-name") != "":
		name := g.PostForm("custom-name")
		if strings.Contains(strings.ToLower(name), "admin") && !u.IsAdmin() {
			name = strings.Replace(name, "admin", "nimda", -1)
		}
		name = common.SoftTruncDisplayWidth(name, 16)
		u2, err := dal.DoUpdateUser(u.ID, "CustomName", name)
		throw(err, "")
		model.IndexUser(u2)
	case g.PostForm("set-avatar") != "":
		throw(common.Err2(writeAvatar(u, g.PostForm("avatar"))), "")
		throw(common.Err2(dal.DoUpdateUser(u.ID, "Avatar", uint32(time.Now().Unix()))), "")
	}

	go dal.DoUpdateUser(u.ID, "DataIP", common.PushIP(u.DataIP, hashIP(g)))
	okok(g)
}

func APIUpdateUserPassword(g *gin.Context) {
	u := throw(dal.GetUserByContext(g), "").(*model.User)
	throw(checkIP(g), "")

	oldPassword := common.SoftTrunc(g.PostForm("old-password"), 32)
	newPassword := common.SoftTrunc(g.PostForm("new-password"), 32)

	throw(len(newPassword) < 3, "new_password_too_short")
	throw(!bytes.Equal(u.PasswordHash, hashPassword(oldPassword)), "old_password_invalid")
	throw(common.Err2(dal.DoUpdateUser(u.ID, "PasswordHash", hashPassword(newPassword))), "")
	okok(g)
}

func APIResetUserPassword(g *gin.Context) {
}

func APIClearInbox(g *gin.Context) {
	u := throw(dal.GetUserByContext(g), "").(*model.User)
	throw(dal.ClearInbox(u.ID), "")
	okok(g)
}
