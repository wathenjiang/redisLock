package redisLock

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis"
)

type RedisLock struct {
	key         string
	value       string
	redisClient *redis.Client
	expiration  time.Duration
	cancelFunc  context.CancelFunc
}

const (
	PubSubPrefix      = "redis_lock_"
	DefaultExpiration = 30
)

func NewRedisLock(redisClient *redis.Client, key string, value string) *RedisLock {
	return &RedisLock{
		key:         key,
		value:       value,
		redisClient: redisClient,
		expiration:  time.Duration(DefaultExpiration) * time.Second}
}

func NewRedisLockWithExpireTime(redisClient *redis.Client, key string, value string, expiration time.Duration) *RedisLock {
	return &RedisLock{
		key:         key,
		value:       value,
		redisClient: redisClient,
		expiration:  expiration}
}

// TryLock try get lock only once, if get the lock return true, else return false
func (lock *RedisLock) TryLock() (bool, error) {
	success, err := lock.redisClient.SetNX(lock.key, lock.value, lock.expiration).Result()
	if err != nil {
		return false, err
	}
	ctx, cancelFunc := context.WithCancel(context.Background())
	lock.cancelFunc = cancelFunc
	lock.renew(ctx)
	return success, nil
}

// Lock blocked until get lock
func (lock *RedisLock) Lock() (bool, error) {
	for {
		success, err := lock.TryLock()
		if err != nil {
			return false, err
		}
		if success {
			return true, nil
		}
		if !success {
			err := lock.subscribeLock()
			if err != nil {
				return false, err
			}
		}
	}
}

// Unlock release the lock
func (lock *RedisLock) Unlock() error {
	script := redis.NewScript(fmt.Sprintf(
		`if redis.call("get", KEYS[1]) == "%s" then return redis.call("del", KEYS[1]) else return 0 end`,
		lock.value))
	runCmd := script.Run(lock.redisClient, []string{lock.key})
	res, err := runCmd.Result()
	if err != nil {
		return err
	}
	if tmp, ok := res.(int64); ok {
		if tmp == 1 {
			lock.cancelFunc() //cancel renew goroutine
			err := lock.publishLock()
			if err != nil {
				return err
			}
			return nil
		}
	}
	err = fmt.Errorf("unlock script fail: %s", lock.key)
	return err
}

// LockWithTimeout blocked until get lock or timeout
func (lock *RedisLock) LockWithTimeout(d time.Duration) (bool, error) {
	timeNow := time.Now()
	for {
		success, err := lock.TryLock()
		if err != nil {
			return false, err
		}
		if success {
			return true, nil
		}
		deltaTime := d - time.Since(timeNow)
		if !success {
			err := lock.subscribeLockWithTimeout(deltaTime)
			if err != nil {
				return false, err
			}
		}
	}
}

func (lock *RedisLock) SpinLock(times int) (bool, error) {
	for i := 0; i < times; i++ {
		success, err := lock.TryLock()
		if err != nil {
			return false, err
		}
		if success {
			return true, nil
		}
	}
	return false, fmt.Errorf("max spin times reached")
}

// subscribeLock blocked until lock is released
func (lock *RedisLock) subscribeLock() error {
	pubSub := lock.redisClient.Subscribe(getPubSubTopic(lock.key))
	_, err := pubSub.Receive()
	if err != nil {
		return err
	}
	<-pubSub.Channel()
	return nil
}

// subscribeLock blocked until lock is released or timeout
func (lock *RedisLock) subscribeLockWithTimeout(d time.Duration) error {
	timeNow := time.Now()
	pubSub := lock.redisClient.Subscribe(getPubSubTopic(lock.key))
	_, err := pubSub.ReceiveTimeout(d)
	if err != nil {
		return err
	}
	deltaTime := time.Since(timeNow) - d
	select {
	case <-pubSub.Channel():
		return nil
	case <-time.After(deltaTime):
		return fmt.Errorf("timeout")
	}
}

// publishLock publish a message about lock is released
func (lock *RedisLock) publishLock() error {
	err := lock.redisClient.Publish(getPubSubTopic(lock.key), "release lock").Err()
	if err != nil {
		return err
	}
	return nil
}

// renew renew the expiration of lock, and can be canceled when call Unlock
func (lock *RedisLock) renew(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(lock.expiration / 3)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				lock.redisClient.Expire(lock.key, lock.expiration).Result()
			}
		}
	}()
}

// getPubSubTopic key -> PubSubPrefix + key
func getPubSubTopic(key string) string {
	return PubSubPrefix + key
}
