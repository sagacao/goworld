package rankredis

import (
	"fmt"
	"io"
	"strings"

	"github.com/garyburd/redigo/redis"
	"github.com/pkg/errors"
	"github.com/sagacao/goworld/engine/netutil"
	"github.com/sagacao/goworld/engine/rank/types"
)

var (
	dataPacker = netutil.MessagePackMsgPacker{}
)

const (
	keyPrefix = "_KV_"
	PASSWORD  = "123456"
)

type redisRank struct {
	c       redis.Conn
	kPrefix string
	passwd  string
}

// OpenRedisRank opens Redis for Rank backend
func OpenRedisRank(url string, prefix string, auth string, dbindex int) (ranktypes.RankEngine, error) {
	c, err := redis.DialURL(url)
	if err != nil {
		return nil, errors.Wrap(err, "redis dail failed")
	}

	if prefix == "" {
		prefix = keyPrefix
	}

	db := &redisRank{
		c:       c,
		kPrefix: prefix,
		passwd:  "",
	}

	if err := db.initialize(dbindex); err != nil {
		panic(errors.Wrap(err, "redis rank initialize failed"))
	}

	return db, nil
}

func (db *redisRank) initialize(dbindex int) error {
	if db.passwd != "" {
		if _, err := db.c.Do("AUTH", db.passwd); err != nil {
			return err
		}
	}

	if dbindex >= 0 {
		if _, err := db.c.Do("SELECT", dbindex); err != nil {
			return err
		}
	}

	return nil
}

func redisDataKey(prefix string) string {
	return fmt.Sprintf("%s:data", prefix)
}

func redisRankKey(prefix string, ckey string) string {
	return fmt.Sprintf("%s:%s", prefix, ckey)
}

func (db *redisRank) Get(key string) (val map[string]string, err error) {
	data, err := redis.Bytes(db.c.Do("HGET", redisDataKey(db.kPrefix), key))
	if err != nil && !strings.Contains(err.Error(), "nil returned") {
		return nil, err
	}

	var user map[string]string
	if data == nil {
		return user, nil
	}

	if err = dataPacker.UnpackMsg(data, &user); err != nil {
		return nil, err
	}
	return user, nil
}

func (db *redisRank) Put(key string, field string, score int64, val interface{}) error {
	var data []byte
	var err error
	data, err = dataPacker.PackMsg(val, data)
	if err != nil {
		return err
	}

	_, err = db.c.Do("ZADD", redisRankKey(db.kPrefix, key), score, field)
	if err != nil {
		return err
	}

	_, err = db.c.Do("HSET", redisDataKey(db.kPrefix), field, data)
	return err
}

func (db *redisRank) GetRank(key string, uid string) (int, error) { // ZREVRANK
	rank, err := redis.Int(db.c.Do("ZREVRANK", redisRankKey(db.kPrefix, key), uid))
	if err != nil {
		return -1, err
	}
	return rank, nil
}

type redisRankIterator struct {
	db      *redisRank
	valuses []string
}

func (it *redisRankIterator) Next() (ranktypes.KVItem, error) {
	if len(it.valuses) == 0 || len(it.valuses)%2 != 0 {
		return ranktypes.KVItem{}, io.EOF
	}

	key := it.valuses[0]
	value := it.valuses[1]
	it.valuses = it.valuses[2:]
	val, err := it.db.Get(key)
	if err != nil {
		return ranktypes.KVItem{}, err
	}
	val["score"] = value

	return ranktypes.KVItem{key, val}, nil
}

func (db *redisRank) List(key string, beginKey string, endKey string) (ranktypes.Iterator, error) {
	data, err := redis.Strings(db.c.Do("ZREVRANGE", redisRankKey(db.kPrefix, key), beginKey, endKey, "withscores"))
	if err != nil {
		return nil, err
	}

	iter := &redisRankIterator{
		db: db,
	}

	for _, v := range data {
		iter.valuses = append(iter.valuses, v)
	}

	return iter, nil
}

func (db *redisRank) Close() {
	db.c.Close()
}

func (db *redisRank) IsConnectionError(err error) bool {
	return err == io.EOF || err == io.ErrUnexpectedEOF
}
