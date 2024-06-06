package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/sniperHW/clustergo/membership"
)

type Admin struct {
	RedisCli        *redis.Client
	heartbeatSha    string
	checkTimeoutSha string
	updateMemberSha string
}

func (cli *Admin) Init() (err error) {
	if cli.heartbeatSha, err = cli.RedisCli.ScriptLoad(context.Background(), ScriptHeartbeat).Result(); err != nil {
		err = fmt.Errorf("error on init ScriptHeartbeat:%s", err.Error())
		return err
	}

	if cli.checkTimeoutSha, err = cli.RedisCli.ScriptLoad(context.Background(), ScriptCheckTimeout).Result(); err != nil {
		err = fmt.Errorf("error on init checkTimeout:%s", err.Error())
		return err
	}

	if cli.updateMemberSha, err = cli.RedisCli.ScriptLoad(context.Background(), ScriptUpdateMember).Result(); err != nil {
		err = fmt.Errorf("error on init updateMember:%s", err.Error())
		return err
	}

	return err
}

func (cli *Admin) UpdateMember(n membership.Node) error {
	jsonBytes, _ := n.Marshal()
	_, err := cli.RedisCli.EvalSha(context.Background(), cli.updateMemberSha, []string{n.Addr.LogicAddr().String()}, "insert_update", string(jsonBytes)).Result()
	return GetRedisError(err)
}

func (cli *Admin) RemoveMember(n membership.Node) error {
	_, err := cli.RedisCli.EvalSha(context.Background(), cli.updateMemberSha, []string{n.Addr.LogicAddr().String()}, "delete").Result()
	return GetRedisError(err)
}

func (cli *Admin) KeepAlive(n membership.Node) error {
	_, err := cli.RedisCli.EvalSha(context.Background(), cli.heartbeatSha, []string{n.Addr.LogicAddr().String()}, 10).Result()
	return GetRedisError(err)
}

func (cli *Admin) CheckTimeout() {
	cli.RedisCli.EvalSha(context.Background(), cli.checkTimeoutSha, []string{}).Result()
}
