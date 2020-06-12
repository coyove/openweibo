package view

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/coyove/iis/common"
	"github.com/coyove/iis/ik"
	"github.com/gin-gonic/gin"
)

const ssoDomain = "https://api.weibo.com"

func reqJSON(c *gin.Context, client *http.Client, req *http.Request) (map[string]interface{}, error) {
	res, err := client.Do(req)
	if err != nil {
		log.Println("[SSO] request failed:", err)
		return nil, err
	}

	body, _ := ioutil.ReadAll(res.Body)
	res.Body.Close()

	m := map[string]interface{}{}
	json.Unmarshal(body, &m)
	return m, nil
}

func getUserInfo(c *gin.Context, accessToken, uid string) (map[string]interface{}, error) {
	req, _ := http.NewRequest("GET", ssoDomain+"/2/users/show.json?access_token="+accessToken+"&uid="+uid, nil)
	m, err := reqJSON(c, http.DefaultClient, req)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func WeiboSSO(c *gin.Context) {
	c.Redirect(302, fmt.Sprintf("%s/oauth2/authorize?client_id=%s&redirect_uri=%s&response_type=code&state=%s",
		ssoDomain, common.Cfg.WeiboOAuthID, common.Cfg.WeiboOAuthRedir, url.QueryEscape(c.Query("redirect"))))
}

func WeiboCallback(c *gin.Context) {
	code := c.Query("code")
	// redirect := c.Query("state")

	if err := c.Query("error"); err != "" {
		c.Set("error", err)
		NotFound(c)
		return
	}

	payload := strings.NewReader(fmt.Sprintf("grant_type=authorization_code&code=%s&client_id=%s&client_secret=%s&redirect_uri=%s",
		code, common.Cfg.WeiboOAuthID, common.Cfg.WeiboOAuthSec, common.Cfg.WeiboOAuthRedir))
	req, _ := http.NewRequest("POST", ssoDomain+"/oauth2/access_token", payload)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	m, err := reqJSON(c, http.DefaultClient, req)
	if err != nil {
		c.Set("error", err.Error())
		NotFound(c)
		return
	}

	accessToken, _ := m["access_token"].(string)
	uid, _ := m["uid"].(string)
	if accessToken == "" || uid == "" {
		c.Set("error", "invalid auth")
		NotFound(c)
		return
	}

	info, err := getUserInfo(c, accessToken, uid)
	if err != nil {
		c.Set("error", err.Error())
		NotFound(c)
		return
	}

	log.Println(info)
	name, _ := info["name"].(string)
	email, _ := info["email"].(string)
	screenName, _ := info["screen_name"].(string)
	avatar, _ := info["profile_image_url"].(string)

	if email == "" {
		email = name + "@sina.com"
	} else if !strings.Contains(email, "@") {
		email += "@sina.com"
	}

	c.Redirect(302, fmt.Sprintf("/user?ott-username=%s&ott-email=%s&ott-avatar=%s&ott-customname=%s&ott=%s",
		url.QueryEscape(name), url.QueryEscape(email), url.QueryEscape(avatar), url.QueryEscape(screenName), ik.MakeOTT(c.ClientIP())))
}
