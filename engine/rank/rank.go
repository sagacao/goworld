package rank

import (
	"time"

	"io"

	"strconv"

	"github.com/sagacao/goworld/engine/async"
	"github.com/sagacao/goworld/engine/config"
	"github.com/sagacao/goworld/engine/gwlog"
	"github.com/sagacao/goworld/engine/rank/backend/rankredis"
	"github.com/sagacao/goworld/engine/rank/backend/rankrediscluster"
	"github.com/sagacao/goworld/engine/rank/types"
)

const (
	_RANK_ASYNC_JOB_GROUP = "_rank"
)

var (
	rankEngine ranktypes.RankEngine
)

// RankGetCallback is type of RANK Get callback
type RankGetCallback func(val map[string]string, err error)

// RankPutCallback is type of RANK Get callback
type RankPutCallback func(err error)

// RankListCallback is type of RANK List callback
type RankListCallback func(items []ranktypes.KVItem, err error)

// RankGetRankCallback is type of RANK GetOrPut callback
type RankGetRankCallback func(rank int, err error)

// Initialize the RANK
//
// Called by game server engine
func Initialize() {
	rankCfg := config.GetRank()
	if rankCfg.Type == "" {
		return
	}

	gwlog.Infof("RANK initializing, config:\n%s", config.DumpPretty(rankCfg))
	assureRankEngineReady()
}

func assureRankEngineReady() (err error) {
	if rankEngine != nil { // connection is valid
		return
	}

	rankCfg := config.GetRank()

	if rankCfg.Type == "redis" {
		var dbindex int = -1
		if rankCfg.DB != "" {
			dbindex, err = strconv.Atoi(rankCfg.DB)
			if err != nil {
				return err
			}
		}
		rankEngine, err = rankredis.OpenRedisRank(rankCfg.Url, rankCfg.Prefix, rankCfg.Auth, dbindex)
	} else if rankCfg.Type == "redis_cluster" {
		rankEngine, err = rankrediscluster.OpenRedisRank(rankCfg.StartNodes.ToList(), rankCfg.Prefix, rankCfg.Auth)
	} else {
		gwlog.Fatalf("RANK type %s is not implemented", rankCfg.Type)
	}
	return
}

// Get gets value of key from RANK, returns in callback
func Get(key string, callback RankGetCallback) {
	var ac async.AsyncCallback
	if callback != nil {
		ac = func(res interface{}, err error) {
			if err != nil {
				callback(nil, err)
			} else {
				callback(res.(map[string]string), nil)
			}
		}
	}
	async.AppendAsyncJob(_RANK_ASYNC_JOB_GROUP, rankRoutine(func() (res interface{}, err error) {
		res, err = rankEngine.Get(key)
		return
	}), ac)
}

func rankRoutine(r func() (res interface{}, err error)) func() (res interface{}, err error) {
	rankroutine := func() (res interface{}, err error) {
		for {
			err := assureRankEngineReady()
			if err == nil {
				break
			} else {
				gwlog.Errorf("RANK engine is not ready: %s", err)
				time.Sleep(time.Second)
			}
		}

		res, err = r()

		if err != nil && rankEngine.IsConnectionError(err) {
			rankEngine.Close()
			rankEngine = nil
		}
		return
	}

	return rankroutine
}

// Put puts key-value item to KVDB, returns in callback
func Put(key string, field string, score int64, val interface{}, callback RankPutCallback) {
	var ac async.AsyncCallback
	if callback != nil {
		ac = func(res interface{}, err error) {
			callback(err)
		}
	}

	async.AppendAsyncJob(_RANK_ASYNC_JOB_GROUP, rankRoutine(func() (res interface{}, err error) {
		err = rankEngine.Put(key, field, score, val)
		return
	}), ac)
}

// List retrives key-value items of specified key range, returns in callback
func List(key string, beginKey string, endKey string, callback RankListCallback) {
	var ac async.AsyncCallback
	if callback != nil {
		ac = func(res interface{}, err error) {
			if err == nil {
				callback(res.([]ranktypes.KVItem), nil)
			} else {
				callback(nil, err)
			}
		}
	}

	async.AppendAsyncJob(_RANK_ASYNC_JOB_GROUP, rankRoutine(func() (res interface{}, err error) {
		it, err := rankEngine.List(key, beginKey, endKey)
		if err != nil {
			return nil, err
		}

		var items []ranktypes.KVItem
		for {
			item, err := it.Next()
			if err == io.EOF {
				break
			}

			if err != nil {
				return nil, err
			}

			items = append(items, item)
		}
		return items, nil
	}), ac)
}

func GetRank(key string, uid string, callback RankGetRankCallback) {
	var ac async.AsyncCallback
	if callback != nil {
		ac = func(res interface{}, err error) {
			if err == nil {
				callback(res.(int), nil)
			} else {
				callback(-1, err)
			}
		}
	}

	async.AppendAsyncJob(_RANK_ASYNC_JOB_GROUP, rankRoutine(func() (res interface{}, err error) {
		res, err = rankEngine.GetRank(key, uid)
		return
	}), ac)
}

// NextLargerKey finds the next key that is larger than the specified key,
// but smaller than any other keys that is larger than the specified key
func NextLargerKey(key string) string {
	return key + "\x00" // the next string that is larger than key, but smaller than any other keys > key
}
