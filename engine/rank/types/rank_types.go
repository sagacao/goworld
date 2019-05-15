package ranktypes

// RankEngine defines the interface of a Rank engine implementation
type RankEngine interface {
	Get(key string) (val map[string]string, err error)
	Put(key string, field string, score int64, val interface{}) (err error)
	List(key string, beginKey string, endKey string) (Iterator, error)
	GetRank(key string, uid string) (val int, err error)
	Close()
	IsConnectionError(err error) bool
}

// Iterator is the interface for iterators for KVDB
//
// Next should returns the next item with error=nil whenever has next item
// otherwise returns KVItem{}, io.EOF
// When failed, returns KVItem{}, error
type Iterator interface {
	Next() (KVItem, error)
}

// KVItem is the type of KVDB item
type KVItem struct {
	Key string
	Val interface{}
}
