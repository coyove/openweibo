package dal

import (
	"fmt"
	"net"
	"strings"

	"github.com/coyove/iis/types"
	"github.com/coyove/sdss/contrib/clock"
	"github.com/tidwall/gjson"
	"go.etcd.io/bbolt"
)

func CreateNote(name string, tag *types.Note, noSort bool) (existed bool, err error) {
	err = Store.Update(func(tx *bbolt.Tx) error {
		if name != "" {
			if _, existed = KSVFirstKeyOfSort1(tx, NoteBK, []byte(name)); existed {
				return nil
			}
		}

		now := clock.UnixMilli()
		tag.CreateUnix = now
		tag.UpdateUnix = now

		ProcessParentChanges(tx, tag, nil, tag.ParentIds)
		UpdateCreator(tx, tag.Creator, tag)
		AppendImage(tx, tag)
		MetricsIncr("create", 1)

		t := KSVFromTag(tag)
		t.NoSort = noSort
		return KSVUpsert(tx, NoteBK, t)
	})
	return
}

func AppendImage(tx *bbolt.Tx, note *types.Note) error {
	bk, err := tx.CreateBucketIfNotExists(append([]byte("image_"), types.Uint64Bytes(note.Id)...))
	if err != nil {
		return err
	}
	if note.ReviewImage != "" {
		if err := bk.Put([]byte(note.ReviewImage), nil); err != nil {
			return err
		}
	}
	if note.Image != "" {
		if err := bk.Put([]byte(note.Image), nil); err != nil {
			return err
		}
	}
	return nil
}

func ListImage(id uint64, deleteBucket bool) (res []string) {
	Store.Update(func(tx *bbolt.Tx) error {
		name := append([]byte("image_"), types.Uint64Bytes(id)...)
		bk := tx.Bucket(name)
		if bk == nil {
			return nil
		}
		c := bk.Cursor()
		for k, _ := c.First(); len(k) > 0; k, _ = c.Next() {
			res = append(res, string(k))
		}
		if deleteBucket {
			return tx.DeleteBucket(name)
		}
		return nil
	})
	return
}

func UpdateCreator(tx *bbolt.Tx, who string, tag *types.Note) error {
	if tx == nil {
		return Store.Update(func(tx *bbolt.Tx) error { return UpdateCreator(tx, who, tag) })
	}
	return KSVUpsert(tx, "creator_"+who, KeySortValue{
		Key:   types.Uint64Bytes(tag.Id),
		Sort0: uint64(clock.UnixMilli()),
		Sort1: []byte(tag.Title),
	})
}

func DeleteCreator(tx *bbolt.Tx, who string, tag *types.Note) error {
	if tx == nil {
		return Store.Update(func(tx *bbolt.Tx) error { return DeleteCreator(tx, who, tag) })
	}
	return KSVDelete(tx, "creator_"+who, types.Uint64Bytes(tag.Id))
}

func FilterInvalidParentIds(ids []uint64) (res []uint64, err error) {
	oks, err := BatchCheckNoteExistences(ids)
	if err != nil {
		return nil, err
	}
	for i := len(ids) - 1; i >= 0; i-- {
		if !oks[i] {
			ids = append(ids[:i], ids[i+1:]...)
		}
	}
	return ids, nil
}

func BatchCheckNoteExistences(v interface{}) (res []bool, err error) {
	ids := convertToBkIDs(v)
	err = Store.View(func(tx *bbolt.Tx) error {
		bk := tx.Bucket([]byte(NoteBK + "_kv"))
		if bk == nil {
			return nil
		}
		for _, kis := range ids {
			res = append(res, len(bk.Get(kis[:])) > 0)
		}
		return nil
	})
	return
}

func convertToBkIDs(v interface{}) (ids [][]byte) {
	switch v := v.(type) {
	case [][]byte:
		ids = v
	case []uint64:
		for _, v := range v {
			ids = append(ids, types.Uint64Bytes(v))
		}
	case []KeySortValue:
		for _, v := range v {
			ids = append(ids, v.Key)
		}
	default:
		panic(v)
	}
	return
}

func BatchGetNotes(v interface{}) (tags []*types.Note, err error) {
	ids := convertToBkIDs(v)
	if len(ids) == 0 {
		return
	}
	err = Store.View(func(tx *bbolt.Tx) error {
		bk := tx.Bucket([]byte(NoteBK + "_kv"))
		if bk == nil {
			return nil
		}
		for _, kis := range ids {
			tag := types.UnmarshalNoteBinary(bk.Get(kis[:]))
			if tag.Valid() {
				tags = append(tags, tag)
			}
		}
		return nil
	})
	return
}

func GetTagRecord(id uint64) (*types.Record, error) {
	var t *types.Record
	err := Store.View(func(tx *bbolt.Tx) error {
		bk := tx.Bucket([]byte("history_kv"))
		if bk == nil {
			return nil
		}
		t = types.UnmarshalRecordBinary(bk.Get(types.Uint64Bytes(id)))
		return nil
	})
	return t, err
}

func GetJsonizedNoteCache(name string) func() gjson.Result {
	var cached gjson.Result
	var ok bool
	return func() gjson.Result {
		if ok {
			return cached
		}
		res, err := GetJsonizedNote(name)
		if err != nil {
			panic(err)
		}
		cached, ok = res, true
		return res
	}
}

func GetJsonizedNote(name string) (gjson.Result, error) {
	t, err := GetNoteByName(name)
	if err != nil {
		return gjson.Result{}, err
	}
	if !t.Valid() {
		return gjson.Result{}, nil
	}
	return gjson.Parse(t.Content), nil
}

func GetNoteByName(name string) (*types.Note, error) {
	if strings.HasPrefix(name, "ns:id:") {
		id, _ := clock.Base40Decode(name[6:])
		return GetNote(id)
	}

	var t *types.Note
	err := Store.View(func(tx *bbolt.Tx) error {
		k, found := KSVFirstKeyOfSort1(tx, NoteBK, []byte(name))
		if found {
			t = types.UnmarshalNoteBinary(tx.Bucket([]byte(NoteBK + "_kv")).Get(k[:]))
		}
		return nil
	})
	return t, err
}

func GetNote(id uint64) (*types.Note, error) {
	var t *types.Note
	err := Store.View(func(tx *bbolt.Tx) error {
		bk := tx.Bucket([]byte(NoteBK + "_kv"))
		if bk == nil {
			return nil
		}
		t = types.UnmarshalNoteBinary(bk.Get(types.Uint64Bytes(id)))
		return nil
	})
	return t, err
}

func ProcessParentChanges(tx *bbolt.Tx, tag *types.Note, old, new []uint64) error {
	k := types.Uint64Bytes(tag.Id)
	bk := tx.Bucket([]byte(NoteBK + "_kv"))
	if bk == nil {
		return nil
	}

	for _, o := range old {
		if types.ContainsUint64(new, o) {
			continue
		}
		ok := types.Uint64Bytes(o)
		ob := bk.Get(ok)
		if len(ob) > 0 {
			bk.Put(ok, types.UpdateNoteBytes(ob, func(b *types.Note) { b.ChildrenCount-- }))
		}
		if err := KSVDelete(tx, fmt.Sprintf("children_%d", o), k); err != nil {
			return err
		}
	}

	now := clock.UnixMilli()
	tt := []byte(tag.Title)
	for _, n := range new {
		if types.ContainsUint64(old, n) {
			// No need to incr 1
		} else {
			nk := types.Uint64Bytes(n)
			nb := bk.Get(nk)
			if len(nb) > 0 {
				bk.Put(nk, types.UpdateNoteBytes(nb, func(b *types.Note) { b.ChildrenCount++ }))
			}
		}
		if err := KSVUpsert(tx, fmt.Sprintf("children_%d", n), KeySortValue{
			Key:   k[:],
			Sort0: uint64(now),
			Sort1: tt,
			Value: nil,
		}); err != nil {
			return err
		}
	}
	return nil
}

func AppendHistory(tagId uint64, user, action string, rejectMsg string, ip net.IP, old *types.Note) error {
	return Store.Update(func(tx *bbolt.Tx) error {
		id := clock.Id()
		tr := (&types.Record{
			Id:         id,
			Action:     int64(action[0]),
			CreateUnix: clock.UnixMilli(),
			Modifier:   user,
			ModifierIP: ip.String(),
			RejectMsg:  rejectMsg,
		})
		switch action {
		case "approve", "reject", "Lock", "Unlock":
			tmp := *old
			tmp.Content = ""
			tmp.ReviewContent = ""
			tr.SetNote(&tmp)
		default:
			tr.SetNote(old)
		}
		k := types.Uint64Bytes(tr.Id)
		KSVUpsert(tx, "history", KeySortValue{
			Key:    k[:],
			Value:  tr.MarshalBinary(),
			NoSort: true,
		})
		KSVUpsert(tx, fmt.Sprintf("history_%s", user), KeySortValue{Key: k[:], NoSort: true})
		return KSVUpsert(tx, fmt.Sprintf("history_%d", tagId), KeySortValue{Key: k[:], NoSort: true})
	})
}
