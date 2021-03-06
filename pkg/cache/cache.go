package cache

import (
	"context"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/bsm/redislock"
	"github.com/go-redis/redis/v8"
	"github.com/pkg/errors"
)

const lockTTLMs = 10000
const lockRetryMs = 100
const lockMaxRetries = lockTTLMs / lockRetryMs

// Cache интерфейс кэша
type Cache interface {
	GetOrSetWhenNotExists(ctx context.Context, key string, f func() ([]byte, error)) ([]byte, error)
}

// NewCache конструирует кэш
func NewCache(minTimeoutSec, maxTimeoutSec int, redisAddress string) Cache {
	rdb := redis.NewClient(&redis.Options{
		Addr: redisAddress,
	})
	locker := redislock.New(rdb)
	return &cache{
		rdb:           rdb,
		locker:        locker,
		minTimeoutSec: minTimeoutSec,
		maxTimeoutSec: maxTimeoutSec,
		keyMutexes:    make(map[string]*sync.Mutex),
	}
}

type cache struct {
	rdb           *redis.Client
	locker        *redislock.Client
	minTimeoutSec int
	maxTimeoutSec int
	mutex         sync.Mutex
	keyMutexes    map[string]*sync.Mutex
}

func (c *cache) GetOrSetWhenNotExists(ctx context.Context, key string, f func() ([]byte, error)) ([]byte, error) {
	c.mutex.Lock()
	m, ok := c.keyMutexes[key]
	if !ok {
		m = &sync.Mutex{}
		c.keyMutexes[key] = m
	}
	c.mutex.Unlock()

	m.Lock()
	defer m.Unlock()

	data, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		if err != redis.Nil {
			log.Printf("[WARN] error get from cache for %s: %v", key, err)
		}

		lockKey := "lock:" + key
		lock, err := c.locker.Obtain(ctx, lockKey, lockTTLMs*time.Millisecond, &redislock.Options{
			RetryStrategy: redislock.LimitRetry(redislock.LinearBackoff(lockRetryMs*time.Millisecond), lockMaxRetries),
		})
		if err != nil {
			log.Printf("[WARN] error obtain lock by key %s: %v", lockKey, err)
		}

		if lock != nil {
			// Блокировка получена.
			// Снова проверим наличие в кэше - другой экземпляр мог добавить.
			defer lock.Release(ctx)

			data, err = c.rdb.Get(ctx, key).Bytes()
			if err == nil {
				// Данные есть в кэше - выходим
				return data, nil
			}
		}

		data, err = f()
		if err != nil {
			return nil, errors.Wrap(err, "f")
		}

		expSeconds := c.minTimeoutSec + rand.Intn(c.maxTimeoutSec-c.minTimeoutSec)
		exp := time.Duration(expSeconds) * time.Second
		err = c.rdb.Set(ctx, key, data, exp).Err()
		if err != nil {
			return nil, errors.Wrap(err, "c.rdb.Set")
		}

		return data, nil
	}

	return data, nil
}
