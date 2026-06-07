package redis

import (
	"context"

	"github.com/redis/go-redis/v9"
	"github.com/sniperHW/clustergo/membership"
)

type Admin struct {
	RedisCli *redis.Client
}

func (cli *Admin) UpdateMember(n membership.Node) error {
	jsonBytes, _ := n.Marshal()
	_, err := updateMember.eval(context.Background(), cli.RedisCli, []string{n.Addr.LogicAddr().String()}, "insert_update", string(jsonBytes))
	return GetRedisError(err)
}

func (cli *Admin) RemoveMember(n membership.Node) error {
	_, err := updateMember.eval(context.Background(), cli.RedisCli, []string{n.Addr.LogicAddr().String()}, "delete")
	return GetRedisError(err)
}

func (cli *Admin) KeepAlive(n membership.Node) error {
	_, err := heartbeat.eval(context.Background(), cli.RedisCli, []string{n.Addr.LogicAddr().String()}, 10)
	return GetRedisError(err)
}

func (cli *Admin) CheckTimeout() {
	checkTimeout.eval(context.Background(), cli.RedisCli, []string{})
}
