package dal

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/coyove/iis/types"
	"github.com/sirupsen/logrus"
)

type S3Error int

func (e S3Error) Error() string {
	return fmt.Sprintf("S3 response: %v", int(e))
}

const ImageCacheDir = "image_cache/"

func imageS3Loader(key, saveTo string) error {
	file := ImageCacheDir + key
	LockKey(file)
	defer UnlockKey(file)

	if buf, err := ioutil.ReadFile(file); err == nil && len(buf) > 0 {
		return ioutil.WriteFile(saveTo, buf, 0777)
	}

	start := time.Now()
	err := func() error {
		resp, err := http.Get("https://nsimages.s3." + types.Config.S3.Region + ".backblazeb2.com/" + key)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			if resp.StatusCode == 404 {
				ioutil.WriteFile(saveTo, []byte("404"), 0777)
				return nil
			}
			return S3Error(resp.StatusCode)
		}

		out, err := os.Create(saveTo)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, resp.Body)
		return err
	}()
	logrus.Infof("load S3 image: %s: %v in %v", key, err, time.Since(start))
	return err
}

func UploadS3(files ...string) (lastErr error) {
	for _, file := range files {
		if file == "" {
			continue
		}
		file = ImageCacheDir + file
		err := func() error {
			LockKey(file)
			defer UnlockKey(file)
			in, err := os.Open(file)
			if err != nil {
				return err
			}
			key := filepath.Base(file)
			_, err = Store.S3.Upload(&s3manager.UploadInput{
				Bucket: aws.String("nsimages"),
				Key:    aws.String(key),
				Body:   in,
			})
			in.Close()
			if err == nil {
				err = os.Remove(file)
			}
			return err
		}()

		logrus.Infof("upload %s to S3: %v", file, err)
		if err != nil {
			lastErr = fmt.Errorf("failed to upload %s to S3: %v", file, err)
		}
	}
	return
}
