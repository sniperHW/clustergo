package redis

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/redis/go-redis/v9"
	"github.com/sniperHW/clustergo/membership"
)

type Membership struct {
	RedisCli      *redis.Client
	memberVersion int64
	aliveVersion  int64
	alive         map[string]struct{}         //健康节点
	members       map[string]*membership.Node //*membership.Node //配置中的节点
	cb            func(membership.MemberInfo)
	once          sync.Once
	closeFunc     context.CancelFunc
	closed        atomic.Bool
}

func GetRedisError(err error) error {
	if err == nil || err.Error() == "redis: nil" {
		return nil
	} else {
		return err
	}
}
