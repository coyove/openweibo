package imagex

import (
	"crypto/sha1"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/coyove/iis/cmd/ch/ident"
	mv "github.com/coyove/iis/cmd/ch/model"
	"github.com/gin-gonic/gin"
)

func Image(g *gin.Context) {
	http.ServeFile(g.Writer, g.Request, filepath.Join("tmp/images", g.Param("path")))
}

func Avatar(g *gin.Context) {
	fn := calcAvatarPath(g.Param("id"))
	if _, err := os.Stat(fn); err != nil {
		http.ServeFile(g.Writer, g.Request, "template/user.png")
		return
	}
	http.ServeFile(g.Writer, g.Request, fn)
}

func GetImage(g *gin.Context) (string, error) {
	image, err := g.FormFile("image")
	if err != nil || image == nil {
		return "", nil
	}

	f, err := image.Open()
	if err != nil {
		return "", err
	}

	defer f.Close()

	ext, hash := strings.ToLower(filepath.Ext(image.Filename)), sha1.Sum([]byte(image.Filename))
	if ext != ".jpg" && ext != ".png" && ext != ".gif" {
		return "", fmt.Errorf("invalid format: %v", ext)
	}

	path := fmt.Sprintf("%s/%s_%x%s",
		time.Now().Format("2006-Jan/02"),
		ident.SafeStringForCompressString12(image.Filename),
		hash[:4],
		ext,
	)

	fn := "tmp/images/" + path
	os.MkdirAll(filepath.Dir(fn), 0755)
	of, err := os.Create(fn)
	if err != nil {
		return "", err
	}

	io.CopyN(of, f, 1024*1024)
	of.Close()
	return path, nil
}

func calcAvatarPath(id string) string {
	return fmt.Sprintf("tmp/avatars/%x/%s", sha1.Sum([]byte(id))[0], id)
}

func GetAvatar(u *mv.User, g *gin.Context) error {
	image, err := g.FormFile("image")
	if err != nil || image == nil {
		return nil
	}

	f, err := image.Open()
	if err != nil {
		return err
	}

	defer f.Close()

	if ext := strings.ToLower(filepath.Ext(image.Filename)); ext != ".jpg" && ext != ".png" && ext != ".gif" {
		return fmt.Errorf("invalid format: %v", ext)
	}

	fn := calcAvatarPath(u.ID)
	os.MkdirAll(filepath.Dir(fn), 0755)
	of, err := os.Create(fn)
	if err != nil {
		return err
	}

	io.CopyN(of, f, 1024*1024)
	of.Close()
	return nil
}
