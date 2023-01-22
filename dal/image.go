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
	"github.com/coyove/sdss/contrib/clock"
	"github.com/sirupsen/logrus"
	"go.etcd.io/bbolt"
)

type S3Error int

func (e S3Error) Error() string {
	return fmt.Sprintf("S3 response: %v", int(e))
}

const ImageCacheDir = "image_cache/"

func imageS3Loader(key, saveTo string) error {
	if buf, err := ioutil.ReadFile(ImageCacheDir + key); err == nil && len(buf) > 0 {
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

func UploadS3(files ...string) {
	for _, file := range files {
		if file == "" {
			continue
		}
		file = ImageCacheDir + file
		err := func() error {
			in, err := os.Open(file)
			if err != nil {
				return err
			}
			defer in.Close()
			key := filepath.Base(file)
			_, err = Store.S3.Upload(&s3manager.UploadInput{
				Bucket: aws.String("nsimages"),
				Key:    aws.String(key),
				Body:   in,
			})
			return err
		}()

		if err == nil {
			err = os.Remove(file)
		}
		logrus.Infof("upload %s to S3: %v", file, err)
	}
}

func StoreImageMetadata(tx *bbolt.Tx, id, noteId uint64) error {
	bk, err := tx.CreateBucketIfNotExists([]byte("images"))
	if err != nil {
		return err
	}
	return bk.Put(types.Uint64Bytes(id), (&types.Image{
		Id:         id,
		NoteId:     id,
		CreateUnix: clock.UnixMilli(),
	}).MarshalBinary())
}
