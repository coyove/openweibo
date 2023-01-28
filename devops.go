package main

import (
	"bufio"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"html"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/types"
	"github.com/coyove/sdss/contrib/bitmap"
	"github.com/coyove/sdss/contrib/clock"
	"github.com/sirupsen/logrus"
	"go.etcd.io/bbolt"
)

func downloadData() {
	downloadWiki := func(p string) ([]string, error) {
		req, _ := http.NewRequest("GET", "https://dumps.wikimedia.org/zhwiki/20230101/"+p, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		rd := bufio.NewReader(bzip2.NewReader(resp.Body))

		var res []string
		for {
			line, err := rd.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					return nil, err
				}
				break
			}
			parts := strings.SplitN(strings.TrimSpace(line), ":", 3)
			x := parts[2]
			if strings.HasPrefix(x, "Category:") ||
				strings.HasPrefix(x, "WikiProject:") ||
				strings.HasPrefix(x, "Wikipedia:") ||
				strings.HasPrefix(x, "File:") ||
				strings.HasPrefix(x, "Template:") {
				continue
			}
			res = append(res, x)
		}
		return res, nil
	}

	for i, p := range strings.Split(`zhwiki-20230101-pages-articles-multistream-index1.txt-p1p187712.bz2
	zhwiki-20230101-pages-articles-multistream-index2.txt-p187713p630160.bz2
	zhwiki-20230101-pages-articles-multistream-index3.txt-p630161p1389648.bz2
	zhwiki-20230101-pages-articles-multistream-index4.txt-p1389649p2889648.bz2
	zhwiki-20230101-pages-articles-multistream-index4.txt-p2889649p3391029.bz2
	zhwiki-20230101-pages-articles-multistream-index5.txt-p3391030p4891029.bz2
	zhwiki-20230101-pages-articles-multistream-index5.txt-p4891030p5596379.bz2
	zhwiki-20230101-pages-articles-multistream-index6.txt-p5596380p7096379.bz2
	zhwiki-20230101-pages-articles-multistream-index6.txt-p7096380p8231694.bz2`, "\n") {
		v, err := downloadWiki(p)
		fmt.Println(p, len(v), err)

		buf := strings.Join(v, "\n")
		ioutil.WriteFile("data"+strconv.Itoa(i), []byte(buf), 0777)
	}

	f, _ := os.Open("out")
	rd := bufio.NewReader(f)
	data := map[string]bool{}
	for i := 0; ; i++ {
		line, err := rd.ReadString('\n')
		if err != nil {
			break
		}
		if data[line] {
			continue
		}
		data[line] = true
	}
	f.Close()
	f, _ = os.Create("out2")
	for k := range data {
		f.WriteString(k)
	}
	f.Close()
}

func rebuildDataFromWiki(count int) {
	f, _ := os.Open("out.gz")
	gr, _ := gzip.NewReader(f)
	rd := bufio.NewReader(gr)

	lines := []string{}
	for i := 0; count <= 0 || i < count; i++ {
		line, err := rd.ReadString('\n')
		if err != nil {
			break
		}
		line = html.UnescapeString(strings.TrimSpace(line))
		lines = append(lines, line)

		if len(lines) > 1000 {
			err := dal.Store.DB.Update(func(tx *bbolt.Tx) error {
				for _, line := range lines {
					i := clock.Id()
					k := types.Uint64Bytes(i)
					now := clock.UnixMilli() - 86400*1000*30
					ksv := dal.KeySortValue{
						Key:   k[:],
						Sort0: uint64(now),
						Sort1: []byte(line),
						Value: (&types.Note{
							Id:         uint64(i),
							Title:      line,
							Creator:    "bulk",
							CreateUnix: now,
							UpdateUnix: now,
						}).MarshalBinary(),
					}
					dal.KSVUpsert(tx, "notes", ksv)
					dal.KSVUpsert(tx, "creator_bulk", dal.KeySortValue{
						Key:   k[:],
						Sort0: uint64(now),
						Sort1: []byte(line),
					})
				}
				return nil
			})

			lines = lines[:0]
			log.Println(i, err)
		}
	}
}

func rebuildIndexFromDB() {
	dal.Store.Saver().Close()

	out := "data/rebuilt"
	os.RemoveAll(out)

	mgr, err := bitmap.NewManager(out, 1024000, *bitmapCacheSize*1e6)
	if err != nil {
		logrus.Fatal("init bitmap manager: ", err)
	}

	dal.Store.DB.View(func(tx *bbolt.Tx) error {
		bk := tx.Bucket([]byte(dal.NoteBK + "_kv"))
		if bk == nil {
			return nil
		}

		c := bk.Cursor()
		i, tot := 0, bk.Sequence()
		for k, v := c.First(); len(k) > 0; k, v = c.Next() {
			note := types.UnmarshalNoteBinary(v)
			h := buildBitmapHashes(note.Title, note.Creator, note.ParentIds)
			mgr.Saver().AddAsync(bitmap.Uint64Key(note.Id), h)
			if i++; i%10000 == 0 {
				logrus.Infof("rebuild bitmap progress: %v / %v", i, tot)
			}
		}
		return nil
	})

	logrus.Infof("rebuild bitmap progress: done, wait for closing...")
	mgr.Saver().Close()

	logrus.Infof("remove current bitmaps: %v", os.RemoveAll("data/index"))
	logrus.Infof("rename rebuilt bitmaps: %v", os.Rename("data/rebuilt", "bitmap_cache/index"))
}

func getWeeklyData() ([]int64, []int64, []int64, []int64, []int64) {
	var a, b, ib, ic, s3 []string
	for i := int64(0); i < 7; i++ {
		d := clock.Unix()/86400 - i
		a = append(a, fmt.Sprintf("daily_create_%d", d))
		b = append(b, fmt.Sprintf("daily_upload_%d", d))
		ic = append(ic, fmt.Sprintf("daily_image_outbound_traffic_req_%d", d))
		ib = append(ib, fmt.Sprintf("daily_image_outbound_traffic_%d", d))
		s3 = append(s3, fmt.Sprintf("daily_s3_download_%d", d))
	}
	av, _ := dal.KVGetInt64s(nil, a)
	bv, _ := dal.KVGetInt64s(nil, b)
	ibv, _ := dal.KVGetInt64s(nil, ib)
	icv, _ := dal.KVGetInt64s(nil, ic)
	s3v, _ := dal.KVGetInt64s(nil, s3)
	return av, bv, ibv, icv, s3v
}
