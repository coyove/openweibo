package dal

import (
	"math/rand"

	"github.com/coyove/iis/types"
	"go.etcd.io/bbolt"
)

func KSVFromUser(user *types.User) KeySortValue {
	return KeySortValue{
		Key:   []byte(user.Id),
		Sort0: uint64(user.CreateUnix),
		Sort1: types.Uint64Bytes(uint64(user.UploadSize)),
		Value: user.MarshalBinary(),
	}
}

func CreateUser(id string, u *types.User) (existed *types.User, err error) {
	err = Store.DB.Update(func(tx *bbolt.Tx) error {
		if bk := tx.Bucket([]byte(UserBK + "_kv")); bk != nil {
			existed = types.UnmarshalUserBinary(bk.Get([]byte(id)))
			if existed.Valid() {
				existed.Session64 = rand.Int63()
				existed.LoginIP = u.CreateIP
				existed.LoginUA = u.LoginUA
				existed.LoginUnix = u.CreateUnix
				existed.LastResetPwd = ""
				return KSVUpsert(tx, UserBK, KSVFromUser(existed))
			}
		}

		MetricsIncr("user:create", 1)
		t := KSVFromUser(u)
		return KSVUpsert(tx, UserBK, t)
	})
	return
}

func GetUser(id string) (u *types.User, err error) {
	err = Store.DB.View(func(tx *bbolt.Tx) error {
		bk := tx.Bucket([]byte(UserBK + "_kv"))
		if bk != nil {
			u = types.UnmarshalUserBinary(bk.Get([]byte(id)))
		}
		return nil
	})
	return
}

func UpdateUser(id string, f func(*types.User) error) (u *types.User, err error) {
	err = Store.DB.Update(func(tx *bbolt.Tx) error {
		if bk := tx.Bucket([]byte(UserBK + "_kv")); bk != nil {
			existed := types.UnmarshalUserBinary(bk.Get([]byte(id)))
			if existed.Valid() {
				if err := f(existed); err != nil {
					return err
				}
				u = existed
				return KSVUpsert(tx, UserBK, KSVFromUser(existed))
			}
		}
		return nil
	})
	return
}
