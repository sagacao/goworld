package kvdbrediscluster

import (
	"io"
	"strings"

	"time"

	redis "github.com/chasex/redis-go-cluster"
	"github.com/pkg/errors"
	"github.com/sagacao/goworld/engine/kvdb/types"
)

const (
	keyPrefix = "_KV_"
)

type redisKVDB struct {
	c       *redis.Cluster
	kPrefix string
	passwd  string
}

// OpenRedisKVDB opens Redis for KVDB backend
func OpenRedisKVDB(startNodes []string, prefix string, auth string) (kvdbtypes.KVDBEngine, error) {
	c, err := redis.NewCluster(&redis.Options{
		StartNodes:   startNodes,
		ConnTimeout:  10 * time.Second, // Connection timeout
		ReadTimeout:  60 * time.Second, // Read timeout
		WriteTimeout: 60 * time.Second, // Write timeout
		KeepAlive:    1,                // Maximum keep alive connecion in each node
		AliveTime:    10 * time.Minute, // Keep alive timeout
	})
	if err != nil {
		return nil, errors.Wrap(err, "redis dail failed")
	}

	if prefix == "" {
		prefix = keyPrefix
	}

	db := &redisKVDB{
		c:       c,
		kPrefix: prefix,
		passwd:  auth,
	}

	return db, nil
}

func (db *redisKVDB) initialize(dbindex int) error {
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

	//keyMatch := db.kPrefix + "*"
	//r, err := redis.Values(db.c.Do("SCAN", "0", "MATCH", keyMatch, "COUNT", 10000))
	//if err != nil {
	//	return err
	//}
	//for {
	//	nextCursor := r[0]
	//	keys, err := redis.Strings(r[1], nil)
	//	if err != nil {
	//		return err
	//	}
	//	for _, key := range keys {
	//		key := key[len(db.kPrefix):]
	//		db.keyTree.ReplaceOrInsert(keyTreeItem{key})
	//	}
	//
	//	if db.isZeroCursor(nextCursor) {
	//		break
	//	}
	//	r, err = redis.Values(db.c.Do("SCAN", nextCursor, "MATCH", keyMatch, "COUNT", 10000))
	//	if err != nil {
	//		return err
	//	}
	//}
	return nil
}

func (db *redisKVDB) isZeroCursor(c interface{}) bool {
	return string(c.([]byte)) == "0"
}

func (db *redisKVDB) Get(key string) (val string, err error) {
	// r, err := db.c.Do("GET", db.kPrefix+key)
	// if err != nil {
	// 	return "", err
	// }
	// if r == nil {
	// 	return "", nil
	// }
	// return string(r.([]byte)), err

	r, err := redis.String(db.c.Do("HGET", db.kPrefix, key))
	if err != nil && !strings.Contains(err.Error(), "nil returned") {
		return "", err
	}
	return r, nil
}

func (db *redisKVDB) Put(key string, val string) error {
	//_, err := db.c.Do("SET", db.kPrefix+key, val)
	_, err := db.c.Do("HSET", db.kPrefix, key, val)
	return err
}

type redisKVDBIterator struct {
	db       *redisKVDB
	leftKeys []string
}

func (it *redisKVDBIterator) Next() (kvdbtypes.KVItem, error) {
	if len(it.leftKeys) == 0 {
		return kvdbtypes.KVItem{}, io.EOF
	}

	key := it.leftKeys[0]
	it.leftKeys = it.leftKeys[1:]
	val, err := it.db.Get(key)
	if err != nil {
		return kvdbtypes.KVItem{}, err
	}

	return kvdbtypes.KVItem{key, val}, nil
}

func (db *redisKVDB) Find(beginKey string, endKey string) (kvdbtypes.Iterator, error) {
	return nil, errors.Errorf("operation not supported on redis")
}

func (db *redisKVDB) Close() {
	db.c.Close()
}

func (db *redisKVDB) IsConnectionError(err error) bool {
	return err == io.EOF || err == io.ErrUnexpectedEOF
}
