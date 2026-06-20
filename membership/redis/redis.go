package redis

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/redis/go-redis/v9"
	"github.com/sniperHW/clustergo"
	"github.com/sniperHW/clustergo/membership"
)

type Membership struct {
	RedisCli      *redis.Client
	MemberKey     string // hash key for members, default "members"
	AliveKey      string // hash key for alive nodes, default "alive"
	Logger        clustergo.Logger
	memberVersion int64
	aliveVersion  int64
	alive         map[string]struct{}         //健康节点
	members       map[string]*membership.Node //*membership.Node //配置中的节点
	cb            func(membership.MemberInfo)
	once          sync.Once
	closeFunc     context.CancelFunc
	closed        atomic.Bool
}

func (m *Membership) getMemberKey() string {
	if m.MemberKey == "" {
		return "members"
	}
	return m.MemberKey
}

func (m *Membership) getAliveKey() string {
	if m.AliveKey == "" {
		return "alive"
	}
	return m.AliveKey
}

func (m *Membership) getMemberVersionKey() string {
	return m.getMemberKey() + "_version"
}

func (m *Membership) getAliveVersionKey() string {
	return m.getAliveKey() + "_version"
}

func GetRedisError(err error) error {
	if err == nil || err.Error() == "redis: nil" {
		return nil
	} else {
		return err
	}
}
