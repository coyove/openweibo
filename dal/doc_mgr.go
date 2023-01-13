package dal

import (
	"fmt"
	"net"

	"github.com/coyove/iis/types"
	"github.com/coyove/sdss/contrib/bitmap"
	"github.com/coyove/sdss/contrib/clock"
	"github.com/tidwall/gjson"
	"go.etcd.io/bbolt"
)

func CreateTag(name string, tag *types.Note) (existed bool, err error) {
	err = TagsStore.Update(func(tx *bbolt.Tx) error {
		if _, existed = KSVFirstKeyOfSort1(tx, "notes", []byte(name)); existed {
			return nil
		}

		now := clock.UnixMilli()
		tag.CreateUnix = now
		tag.UpdateUnix = now

		ProcessTagParentChanges(tx, tag, nil, tag.ParentIds)

		UpdateTagCreator(tx, tag)
		return KSVUpsert(tx, "notes", KSVFromTag(tag))
	})
	return
}

func UpdateTagCreator(tx *bbolt.Tx, tag *types.Note) error {
	k := bitmap.Uint64Key(tag.Id)
	return KSVUpsert(tx, "creator_"+tag.Creator, KeySortValue{
		Key:   k[:],
		Sort0: uint64(clock.UnixMilli()),
		Sort1: []byte(tag.Title),
	})
}

func BatchGetNotes(v interface{}) (tags []*types.Note, err error) {
	var ids []bitmap.Key
	switch v := v.(type) {
	case []bitmap.Key:
		ids = v
	case []uint64:
		for _, v := range v {
			ids = append(ids, bitmap.Uint64Key(v))
		}
	case []KeySortValue:
		for _, v := range v {
			ids = append(ids, bitmap.BytesKey(v.Key))
		}
	default:
		panic(v)
	}
	err = TagsStore.View(func(tx *bbolt.Tx) error {
		bk := tx.Bucket([]byte("notes"))
		if bk == nil {
			return nil
		}
		for _, kis := range ids {
			tag := types.UnmarshalTagBinary(bk.Get(kis[:]))
			if tag.Valid() {
				tags = append(tags, tag)
			}
		}
		return nil
	})
	return
}

func GetTagRecord(id bitmap.Key) (*types.NoteRecord, error) {
	var t *types.NoteRecord
	err := TagsStore.View(func(tx *bbolt.Tx) error {
		bk := tx.Bucket([]byte("history"))
		if bk == nil {
			return nil
		}
		t = types.UnmarshalTagRecordBinary(bk.Get(id[:]))
		return nil
	})
	return t, err
}

func GetTagJSONDescription(name string) (gjson.Result, error) {
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
	err := TagsStore.View(func(tx *bbolt.Tx) error {
		k, found := KSVFirstKeyOfSort1(tx, "notes", []byte(name))
		if found {
			t = types.UnmarshalTagBinary(tx.Bucket([]byte("notes")).Get(k[:]))
		}
		return nil
	})
	return t, err
}

func GetTag(id uint64) (*types.Note, error) {
	var t *types.Note
	err := TagsStore.View(func(tx *bbolt.Tx) error {
		bk := tx.Bucket([]byte("notes"))
		if bk == nil {
			return nil
		}
		k := bitmap.Uint64Key(id)
		t = types.UnmarshalTagBinary(bk.Get(k[:]))
		return nil
	})
	return t, err
}

func ProcessTagParentChanges(tx *bbolt.Tx, tag *types.Note, old, new []uint64) error {
	k := bitmap.Uint64Key(tag.Id)
	for _, o := range old {
		if err := KSVDelete(tx, fmt.Sprintf("children_%d", o), k[:]); err != nil {
			return err
		}
	}
	now := clock.UnixMilli()
	for _, n := range new {
		if err := KSVUpsert(tx, fmt.Sprintf("children_%d", n), KeySortValue{
			Key:   k[:],
			Sort0: uint64(now),
			Sort1: []byte(tag.Title),
			Value: nil,
		}); err != nil {
			return err
		}
	}
	return nil
}

func ProcessTagHistory(tagId uint64, user, action string, ip net.IP, old *types.Note) error {
	return TagsStore.Update(func(tx *bbolt.Tx) error {
		id := clock.Id()
		tr := (&types.NoteRecord{
			Id:         id,
			Action:     int64(action[0]),
			CreateUnix: clock.UnixMilli(),
			Modifier:   user,
			ModifierIP: ip.String(),
		})
		tr.SetNote(old)
		k := bitmap.Uint64Key(tr.Id)
		KSVUpsert(tx, "history", KeySortValue{
			Key:    k[:],
			Value:  tr.MarshalBinary(),
			NoSort: true,
		})
		KSVUpsert(tx, fmt.Sprintf("history_%s", user), KeySortValue{Key: k[:], NoSort: true})
		return KSVUpsert(tx, fmt.Sprintf("history_%d", tagId), KeySortValue{Key: k[:], NoSort: true})
	})
}

type int64Heap struct {
	data []int64
}

func (h *int64Heap) Len() int {
	return len(h.data)
}

func (h *int64Heap) Less(i, j int) bool {
	return h.data[i] < h.data[j]
}

func (h *int64Heap) Swap(i, j int) {
	h.data[i], h.data[j] = h.data[j], h.data[i]
}

func (h *int64Heap) Push(x interface{}) {
	h.data = append(h.data, x.(int64))
}

func (h *int64Heap) Pop() interface{} {
	old := h.data
	n := len(old)
	x := old[n-1]
	h.data = old[:n-1]
	return x
}
