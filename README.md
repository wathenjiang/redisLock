# README

## 1. Quick Start

Install redisLock:

```
go get github.com/Spongecaptain/redisLock
```

Create redis client:

```go
import(
	"github.com/go-redis/redis"
)
var redisClient = redis.NewClient(&redis.Options{
	Addr:     "localhost:6379",
	Password: "", // no password set
	DB:       0,  // use default DB
})
```

Create redisLock:

```go
key := "reids-lock-key"
value := "redis-lock-value"
lock := redisLock.NewRedisLock(redisClient, key, value)

err := lock.Lock()
if err != nil {
  fmt.Println(err.Error())
  return
}
fmt.Println("get redis lock success")
defer func() {
  err = lock.Unlock()
  if err != nil {
    fmt.Println(err.Error())
    return
  }
  fmt.Println("release redis lock success")
}()
```

## 2. Feature

redisLock supports the following features:

-  Implements Atomic by Lua scripts
-  Achieve efficient communication by [Redis Pub/Sub](https://redis.io/topics/pubsub)
-  Avoid deadlock by [Redis EXPIRE](https://redis.io/commands/expire)
-  Avoid concurrency issues by automatic renew

But here are the **unsupported** features:

- Reentrant mutex is not supported, just like [sync.muetx](https://golang.org/pkg/sync/)
- Fairness of lock is not supported, may causes starving problem

