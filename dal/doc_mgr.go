package dal

import (
	"fmt"
	"net"

	"github.com/coyove/iis/types"
	"github.com/coyove/sdss/contrib/clock"
	"github.com/tidwall/gjson"
	"go.etcd.io/bbolt"
)

func CreateNote(name string, tag *types.Note) (existed bool, err error) {
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
		UpdateCreator(tx, tag)
		return KSVUpsert(tx, NoteBK, KSVFromTag(tag))
	})
	return
}

func UpdateCreator(tx *bbolt.Tx, tag *types.Note) error {
	return KSVUpsert(tx, "creator_"+tag.Creator, KeySortValue{
		Key:   types.Uint64Bytes(tag.Id),
		Sort0: uint64(clock.UnixMilli()),
		Sort1: []byte(tag.Title),
	})
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
			bk.Put(ok, types.IncrNoteChildrenCountBinary(ob, -1))
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
				bk.Put(nk, types.IncrNoteChildrenCountBinary(nb, 1))
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

func AppendHistory(tagId uint64, user, action string, ip net.IP, old *types.Note) error {
	return Store.Update(func(tx *bbolt.Tx) error {
		id := clock.Id()
		tr := (&types.Record{
			Id:         id,
			Action:     int64(action[0]),
			CreateUnix: clock.UnixMilli(),
			Modifier:   user,
			ModifierIP: ip.String(),
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
