package redis

import (
	"context"
	"fmt"
	"sync"

	"github.com/redis/go-redis/v9"
)

type Admin struct {
	RedisCli        *redis.Client
	heartbeatSha    string
	checkTimeoutSha string
	updateMemberSha string
	once            sync.Once
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

func (cli *Admin) UpdateMember(n *Node) error {
	jsonBytes, _ := n.Marshal()
	_, err := cli.RedisCli.EvalSha(context.Background(), cli.updateMemberSha, []string{n.LogicAddr}, "insert_update", string(jsonBytes)).Result()
	return GetRedisError(err)
}

func (cli *Admin) RemoveMember(n *Node) error {
	_, err := cli.RedisCli.EvalSha(context.Background(), cli.updateMemberSha, []string{n.LogicAddr}, "delete").Result()
	return GetRedisError(err)
}

func (cli *Admin) KeepAlive(n *Node) error {
	_, err := cli.RedisCli.EvalSha(context.Background(), cli.heartbeatSha, []string{n.LogicAddr}, 10).Result()
	return GetRedisError(err)
}

func (cli *Admin) CheckTimeout() {
	cli.RedisCli.EvalSha(context.Background(), cli.checkTimeoutSha, []string{}).Result()
}
