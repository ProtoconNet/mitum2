package base

import "github.com/ProtoconNet/mitum2/util"

var objcache *util.GCacheObjectPool

func init() {
	objcache = util.NewGCacheObjectPool(1 << 13) //nolint:gomnd //...
}
