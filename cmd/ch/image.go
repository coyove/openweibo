package main

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/coyove/ch"
)

func getImageLocalTmpPath(filename string, v []byte) (localPath string, displayPath string) {
	key := mgr.MakeKey(v)
	x := [16]byte{}
	copy(x[:], key[:])
	config.Blk.Encrypt(x[:], x[:])

	displayPath = hex.EncodeToString(x[:]) + filepath.Ext(filename)

	localPath = fmt.Sprintf("tmp/images/%d", key[0])
	os.MkdirAll(localPath, 0700)
	localPath += "/" + key.String()
	return
}

func extractImageKey(key string) (ch.Key, string, error) {
	k := ch.Key{}
	key = filepath.Base(key)
	buf, _ := hex.DecodeString(key)
	if len(buf) != 16 {
		return k, "", fmt.Errorf("invalid key: %v", key)
	}

	config.Blk.Decrypt(buf, buf)
	copy(k[:], buf)
	return k, fmt.Sprintf("tmp/images/%d/%s", k[0], k.String()), nil
}

func uploadLocalImages() {
	for {
		filepath.Walk("tmp/images", func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			log.Println("[upload]", path)

			k := ch.Key{}
			if k.FromString(filepath.Base(path)) != nil {
				os.Remove(path)
				return nil
			}

			v, err := ioutil.ReadFile(path)
			if err != nil {
				return nil
			}

			if k2, err := mgr.Put(v); err != nil {
				log.Println("[upload] upload error:", err)
				return nil
			} else if k2 != k.String() {
				log.Println("[upload] missed keys:", k2, k)
				return nil
			}

			log.Println("[upload] finished:", path, len(v))
			os.Remove(path)

			keystr := k.String()
			cachepath := fmt.Sprintf("tmp/cache/%s/%s", keystr[:2], keystr)
			ioutil.WriteFile(cachepath, v, 0700)

			time.Sleep(time.Second)
			return nil
		})
		time.Sleep(time.Second * 10)
	}
}
