package media

import (
	"time"

	"github.com/jellydator/ttlcache/v3"
	"github.com/ttab/ttninjs"
	"golang.org/x/sync/singleflight"
)

// CacheOptions configures the TTNINJS document cache.
type CacheOptions struct {
	// TTL is how long a successful lookup is kept in the cache.
	// Defaults to 60 seconds.
	TTL time.Duration
	// Capacity is the maximum number of entries in the cache.
	// Defaults to 2048.
	Capacity uint64
}

type mediaCache struct {
	store *ttlcache.Cache[string, ttninjs.Document]
	group singleflight.Group
}

func newCache(opts CacheOptions) *mediaCache {
	if opts.TTL == 0 {
		opts.TTL = 60 * time.Second
	}

	if opts.Capacity == 0 {
		opts.Capacity = 2048
	}

	store := ttlcache.New(
		ttlcache.WithTTL[string, ttninjs.Document](opts.TTL),
		ttlcache.WithCapacity[string, ttninjs.Document](opts.Capacity),
	)

	go store.Start()

	return &mediaCache{store: store}
}
