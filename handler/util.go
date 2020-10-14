package handler

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
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

var (
	rxShortID = regexp.MustCompile(`\d{4}-?\d{4}-?\d{4}`)
	throw     = middleware.ThrowIf
)

func err2(v interface{}, e error) error {
	return e
}

func okok(g *gin.Context, tmp ...string) {
	g.String(200, "ok")
	if len(tmp) > 0 {
		g.Writer.Write([]byte(":"))
		for _, t := range tmp {
			g.Writer.WriteString(t)
		}
	}
}

func NotFound(g *gin.Context) {
	if path := strings.TrimPrefix(g.Request.URL.Path, "/"); rxShortID.MatchString(path) {
		id := strings.Replace(path, "-", "", -1)
		shortID, _ := strconv.ParseInt(id, 10, 64)
		aid, err := ik.ParseShortId(shortID)
		if err != nil {
			log.Println(err)
			goto SKIP
		}
		buf, err := dal.ModKV().WeakGet("fw/" + fmt.Sprint(aid))
		if err != nil || len(buf) == 0 {
			log.Println(err)
			goto SKIP
		}
		tmp, _ := ik.StringifyShortId(aid)
		Static(g, string(buf), tmp)
		return
	}

SKIP:
	p := struct {
		Accept bool
		Msg    string
	}{
		g.GetBool("need-accept"),
		g.GetString("error"),
	}
	g.HTML(404, "error.html", p)
}

func getUser(g *gin.Context) *model.User {
	u, _ := g.Get("user")
	u2, _ := u.(*model.User)
	return u2
}

type ReplyView struct {
	UUID    string
	ReplyTo string
}

func makeReplyView(g *gin.Context, reply string) ReplyView {
	r := ReplyView{}
	r.UUID = strconv.FormatInt(time.Now().UnixNano(), 16)
	r.ReplyTo = reply
	return r
}

func makeCheckpoints(g *gin.Context) []string {
	r := []string{}

	for y, m := time.Now().Year(), int(time.Now().Month()); ; {
		m--
		if m == 0 {
			y, m = y-1, 12
		}

		if y < 2020 {
			break
		}

		r = append(r, fmt.Sprintf("%04d-%02d", y, m))

		if y == 2020 && m == 1 {
			// 2020-01 is the genesis
			break
		}

		if len(r) >= 6 {
			// return 6 checkpoints (months) at most
			break
		}
	}

	return r
}

func lastMonth(v string) string {
	t, err := time.Parse("2006-01", v)
	if err != nil {
		return ""
	}
	if t.Month() == 1 {
		return fmt.Sprintf("%04d-12", t.Year()-1)
	}
	return fmt.Sprintf("%04d-%02d", t.Year(), t.Month()-1)
}

func redirectVisitor(g *gin.Context) {
	g.Redirect(302, "/?redirect="+url.QueryEscape(g.Request.URL.String()))
}

// var captchaClient = &http.Client{Timeout: 500 * time.Millisecond}

func checkIP(g *gin.Context) string {
	if u, _ := g.Get("user"); u != nil {
		if u.(*model.User).IsMod() {
			return ""
		}
	}
	if !g.GetBool("ip-ok") {
		return fmt.Sprintf("cooldown`%.1fs", float64(common.Cfg.Cooldown)-g.GetFloat64("ip-ok-remain"))
	}
	return ""
}

func sanUsername(id string) string {
	return common.SafeStringForCompressString(id)
}

func hashPassword(password string) []byte {
	pwdHash := hmac.New(sha256.New, common.Cfg.KeyBytes)
	pwdHash.Write([]byte(password))
	return pwdHash.Sum(nil)
}

func checkCaptcha(g *gin.Context) string {
	var (
		answer            = common.SoftTrunc(g.PostForm("answer"), 6)
		uuid              = common.SoftTrunc(g.PostForm("uuid"), 32)
		tokenbuf, tokenok = ik.ParseToken(g, uuid)
		response          = g.PostForm("response")
		challengePassed   bool
	)

	if ret := checkIP(g); ret != "" {
		return ret
	}

	if response != "" {
		// 		p := fmt.Sprintf("response=%s&secret=%s", response, common.Cfg.HCaptchaSecKey)
		// 		resp, err := captchaClient.Post("https://hcaptcha.com/siteverify", "application/x-www-form-urlencoded", strings.NewReader(p))
		// 		if err != nil {
		// 			log.Println("hcaptcha server failure:", err)
		// 			return "guard/failed-captcha"
		// 		}
		// 		buf, _ := ioutil.ReadAll(resp.Body)
		// 		resp.Body.Close()
		// 		m := map[string]interface{}{}
		// 		json.Unmarshal(buf, &m)
		//
		// 		if ok, _ := m["success"].(bool); ok {
		// 			return ""
		// 		}
		//
		// 		return "guard/failed-captcha:" + fmt.Sprint(m["error-codes"])
	}

	if !tokenok {
		return "expired_session"
	}

	if len(answer) == 4 {
		challengePassed = true
		for i := range answer {
			a := answer[i]
			if a >= 'A' && a <= 'Z' {
				a = a - 'A' + 'a'
			}

			if a != "0123456789acefhijklmnpqrtuvwxyz"[tokenbuf[i]%10] &&
				a != "oiz3asg7b9acefhijklmnpqrtuvwxyz"[tokenbuf[i]%10] {
				challengePassed = false
				break
			}
		}
	}

	if !challengePassed {
		log.Println(g.MustGet("ip"), "challenge failed")
		return "captcha_failed"
	}

	return ""
}

func genSession() string {
	p := [12]byte{}
	rand.Read(p[:])
	for i := range p {
		if p[i] == 0 {
			p[i] = 1
		}
	}
	return base64.URLEncoding.EncodeToString(p[:])
}

func writeImageReader(u *model.User, imageName string, hash uint64, dec io.Reader, ct string, large bool) (string, error) {
	path := "tmp/images/"
	fn := fmt.Sprintf("%016x", hash)

	if imageName != "" {
		imageName = filepath.Base(imageName)
		imageName = strings.TrimSuffix(imageName, filepath.Ext(imageName))
		fn += "_" + common.SafeStringForCompressString(imageName) + "_" + u.ID
	} else {
		fn += "_" + u.ID
	}

	if dal.S3 != nil {
		fn += mimeToExt(ct)

		if large {
			of, err := ioutil.TempFile("", "")
			if err != nil {
				return "", err
			}
			size, err := io.Copy(of, dec)
			if err != nil {
				of.Close()
				return "", err
			}
			go func() {
				of.Seek(0, 0)
				err := dal.S3.Put(fn, ct, of)
				of.Close()
				log.Println("Large upload:", fn, size, "S3 err:", err, "purge:", os.Remove(of.Name()))
			}()
			x := "LOCAL:" + fn
			return "CHECK(" + common.Cfg.MediaDomain + "/" + fn + ")-" + x, nil
		}

		return "LOCAL:" + fn, dal.S3.Put(fn, ct, dec)
	}

	if ct == "image/gif" {
		fn += "_mime~gif"
	}

	of, err := os.OpenFile(path+fn, os.O_CREATE|os.O_WRONLY, 0777)
	if err != nil {
		return "", err
	}
	_, err = io.Copy(of, dec)
	of.Close()

	// if n > 100*1024 { // thumbnail
	// 	if err := writeThumbnail(path+fn+"@thumb", path+fn, 200); err != nil {
	// 		log.Println("WriteImage:", err)
	// 	}
	// }

	return "LOCAL:" + fn, err
}

func mimeToExt(mime string) string {
	switch mime {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	default:
		return ""
	}
}

func writeAvatar(u *model.User, image string) (string, error) {
	const max = 150 * 1024 * 4 / 3

	image = image[strings.Index(image, ",")+1:]
	if len(image) > max {
		image = image[:max]
	}
	buf, err := base64.StdEncoding.DecodeString(image)
	if err != nil {
		return "", err
	}

	hash := u.IDHash()
	path := "tmp/images/"
	fn := fmt.Sprintf("%016x@%s", hash, u.ID)

	if dal.S3 != nil {
		ct := http.DetectContentType(buf)
		return fn, dal.S3.Put(fn, ct, bytes.NewReader(buf))
	}

	of, err := os.OpenFile(path+fn, os.O_CREATE|os.O_WRONLY, 0777)
	if err != nil {
		return "", err
	}
	_, err = of.Write(buf)
	of.Close()
	return fn, err
}

func handlePollContent(a *model.Article) {
	lines := strings.Split(a.Content, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			lines = append(lines[:i], lines[i+1:]...)
			continue
		}
		lines[i] = line
	}
	if len(lines) < 2 {
		return
	}
	if len(lines) > 6+1 {
		lines = lines[:6+1]
	}

	a.Extras = map[string]string{}
	a.Extras["poll_title"] = lines[0]
	a.Extras["poll_options"] = strconv.Itoa(len(lines) - 1)

	for title := lines[0]; ; {
		idx := strings.LastIndex(title, " ")
		if idx == -1 {
			break
		}
		kv := title[idx+1:]
		if idx := strings.Index(kv, "="); strings.HasPrefix(kv, "-") && idx > -1 {
			a.Extras["poll_"+kv[1:idx]] = kv[idx+1:]
		}
		title = title[:idx]
	}

	for i := 1; i < len(lines); i++ {
		a.Extras["poll_choice_"+strconv.Itoa(i)] = lines[i]
	}

	a.Content = lines[0]
}
