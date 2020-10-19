package handler

import (
	"bytes"
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

func APISignup(g *gin.Context) {
	var (
		ip       = hashIP(g)
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
		admin, err := dal.GetUser(common.Cfg.AdminName)
		throw(err, "")
		throw(admin != nil, "duplicated_id")
	}

	u := &model.User{}
	u.ID = username
	u.Session = genSession()
	u.Email = email
	u.PasswordHash = hashPassword(password)
	u.DataIP = ip
	u.TSignup = uint32(time.Now().Unix())
	u.TLogin = u.TSignup

	tok := ik.MakeUserToken(u)
	throw(err2(dal.DoUpdateUser(&dal.UpdateUserRequest{
		Signup:       true,
		ID:           u.ID,
		Session:      aws.String(u.Session),
		Email:        aws.String(u.Email),
		PasswordHash: &u.PasswordHash,
		DataIP:       aws.String(u.DataIP),
		TSignup:      aws.Uint32(u.TSignup),
		TLogin:       aws.Uint32(u.TLogin),
	})), "")
	g.SetCookie("id", tok, 365*86400, "", "", false, false)
	okok(g)
}

func APILogin(g *gin.Context) {
	throw(checkIP(g), "")

	u, _ := dal.GetUser(sanUsername(g.PostForm("username")))
	throw(u, "invalid_id_password")
	throw(!bytes.Equal(u.PasswordHash, hashPassword(g.PostForm("password"))), "invalid_id_password")

	// u.Session = genSession()
	u.TLogin = uint32(time.Now().Unix())

	if ips := append(strings.Split(u.DataIP, ","), hashIP(g)); len(ips) > 3 {
		u.DataIP = strings.Join(ips[len(ips)-3:], ",")
	} else {
		u.DataIP = strings.Join(ips, ",")
	}

	tok := ik.MakeUserToken(u)
	throw(err2(dal.DoUpdateUser(&dal.UpdateUserRequest{
		ID:      u.ID,
		Session: aws.String(u.Session),
		DataIP:  aws.String(u.DataIP),
		TLogin:  aws.Uint32(u.TLogin),
	})), "")

	ttl := 0
	if g.PostForm("remember") != "" {
		ttl = 365 * 86400
	}
	g.SetCookie("id", tok, ttl, "", "", false, false)
	okok(g)
}

func APIUserKimochi(g *gin.Context) {
	u := throw(dal.GetUserByContext(g), "").(*model.User)

	k, _ := strconv.Atoi(g.PostForm("k"))
	if k < 0 || k > 44 {
		k = 25
	}

	throw(err2(dal.DoUpdateUser(&dal.UpdateUserRequest{ID: u.ID, Kimochi: aws.Uint8(byte(k))})), "")
	okok(g)
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
		dal.DoUpdateUser(&dal.UpdateUserRequest{
			ID:      u.ID,
			Session: aws.String(genSession()),
		})
		u = &model.User{}
		g.SetCookie("id", ik.MakeUserToken(u), 365*86400, "", "", false, false)
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
		throw(err2(dal.DoUpdateUser(&dal.UpdateUserRequest{
			ID:    u.ID,
			Email: aws.String(common.SoftTrunc(g.PostForm("email"), 256)),
		})), "")
	case g.PostForm("set-autonsfw") != "":
		throw(err2(dal.DoUpdateUser(&dal.UpdateUserRequest{
			ID:               u.ID,
			S_AutoExpandNSFW: aws.Bool(g.PostForm("autonsfw") != ""),
		})), "")
	case g.PostForm("set-foldimg") != "":
		throw(err2(dal.DoUpdateUser(&dal.UpdateUserRequest{
			ID:           u.ID,
			S_FoldImages: aws.Bool(g.PostForm("foldimg") != ""),
		})), "")
	case g.PostForm("set-hl") != "":
		throw(err2(dal.DoUpdateUser(&dal.UpdateUserRequest{
			ID:                u.ID,
			S_HideGeolocation: aws.Bool(g.PostForm("hl") != ""),
		})), "")
	case g.PostForm("set-hide-likes-timeline") != "":
		throw(err2(dal.DoUpdateUser(&dal.UpdateUserRequest{
			ID:                  u.ID,
			S_HideLikesTimeline: aws.Bool(g.PostForm("hide-likes-timeline") != ""),
		})), "")
	case g.PostForm("set-hide-likes") != "":
		throw(err2(dal.DoUpdateUser(&dal.UpdateUserRequest{
			ID:          u.ID,
			S_HideLikes: aws.Bool(g.PostForm("hide-likes") != ""),
		})), "")
	case g.PostForm("set-mfcm") != "":
		throw(err2(dal.DoUpdateUser(&dal.UpdateUserRequest{
			ID:              u.ID,
			S_KnownMentions: aws.Bool(g.PostForm("mfcm") != ""),
		})), "")
	case g.PostForm("set-description") != "":
		throw(err2(dal.DoUpdateUser(&dal.UpdateUserRequest{
			ID:            u.ID,
			S_Description: aws.String(common.SoftTrunc(g.PostForm("description"), 512)),
		})), "")
	case g.PostForm("set-apisession") != "":
		apiSession := "api+" + genSession()
		u.Session = apiSession
		apiToken := ik.MakeUserToken(u)
		throw(err2(dal.DoUpdateUser(&dal.UpdateUserRequest{
			ID:         u.ID,
			S_APIToken: aws.String(apiToken),
		})), "")
		okok(g, apiToken)
		return
	case g.PostForm("set-fw-accept") != "":
		throw(err2(dal.DoUpdateUser(&dal.UpdateUserRequest{
			ID:          u.ID,
			FollowApply: aws.Bool(g.PostForm("fw-accept") != ""),
		})), "")
	case g.PostForm("set-custom-name") != "":
		name := g.PostForm("custom-name")
		if strings.Contains(strings.ToLower(name), "admin") && !u.IsAdmin() {
			name = strings.Replace(name, "admin", "nimda", -1)
		}
		name = common.SoftTruncDisplayWidth(name, 16)
		u2, err := dal.DoUpdateUser(&dal.UpdateUserRequest{ID: u.ID, CustomName: &name})
		throw(err, "")
		g.Writer.Header().Add("X-Result",
			url.PathEscape(middleware.RenderTemplateString("display_name.html", u2)))
		g.Writer.Header().Add("X-Custom-Name", url.PathEscape(name))
		model.IndexUser(&u2, true)
	case g.PostForm("set-avatar") != "":
		throw(err2(writeAvatar(u, g.PostForm("avatar"))), "")
		throw(err2(dal.DoUpdateUser(&dal.UpdateUserRequest{
			ID:     u.ID,
			Avatar: aws.Uint32(uint32(time.Now().Unix())),
		})), "")

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
	okok(g)
}

func APIUpdateUserPassword(g *gin.Context) {
	u := throw(dal.GetUserByContext(g), "").(*model.User)
	throw(checkIP(g), "")

	oldPassword := common.SoftTrunc(g.PostForm("old-password"), 32)
	newPassword := common.SoftTrunc(g.PostForm("new-password"), 32)

	throw(len(newPassword) < 3, "new_password_too_short")
	throw(!bytes.Equal(u.PasswordHash, hashPassword(oldPassword)), "old_password_invalid")

	ph := hashPassword(newPassword)
	throw(err2(dal.DoUpdateUser(&dal.UpdateUserRequest{ID: u.ID, PasswordHash: &ph})), "")
	okok(g)
}

func APIResetUserPassword(g *gin.Context) {
	// throw(checkIP(g), "")

	// username := sanUsername(g.PostForm("username"))
	// email := common.SoftTrunc(g.PostForm("email"), 64)

	// u, err := dal.GetUser(username)
	// throw(err, "")
	// throw(u.Email != email, "")

	// newPassword, _ := ioutil.ReadAll(io.LimitReader(rand.Reader, 8))
	// hp := hashPassword(hex.EncodeToString(newPassword))
	// _, err = dal.DoUpdateUser(&dal.UpdateUserRequest{ID: u.ID, PasswordHash: &hp})
	// throw(err, "")
	// throw(common.SendMail(email,
	// 	"Password Reset",
	// 	fmt.Sprintf("Username: %q, New Password: %v", u.ID, hex.EncodeToString(newPassword))), "")
	// okok(g)
}

func APIClearInbox(g *gin.Context) {
	u := throw(dal.GetUserByContext(g), "").(*model.User)
	throw(dal.ClearInbox(u.ID), "")
	okok(g)
}
