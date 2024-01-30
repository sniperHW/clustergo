package client

import (
	"context"
	"sync"

	"github.com/go-redis/redis"
	"github.com/sniperHW/clustergo"
	"github.com/sniperHW/clustergo/membership"
)

type Node struct {
	LogicAddr string
	NetAddr   string
	Export    bool
	Available bool
}

type Members struct {
	Nodes map[string]*Node
}

type Client struct {
	sync.Mutex
	version             int
	alive               map[string]struct{}        //健康节点
	members             map[string]membership.Node //配置中的节点
	once                sync.Once
	LogicAddr           string
	Logger              clustergo.Logger
	cb                  func(membership.MemberInfo)
	closeFunc           context.CancelFunc
	closed              bool
	redisCli            *redis.Client
	scriptGetMembersSha string
	scriptHeartbeatSha  string
}

func (cli *Client) initScriptSha() (err error) {
	//if cli.scriptGetSha, err = cli.redisCli.ScriptLoad(scriptGet).Result(); err != nil {
	//	err = fmt.Errorf("error on init scriptGet:%s", err.Error())
	//	return err
	//}

	//if cli.scriptHeartbeatSha, err = cli.redisCli.ScriptLoad(scriptHeartbeat).Result(); err != nil {
	//	err = fmt.Errorf("error on init scriptGet:%s", err.Error())
	//	return err
	//}
	return err
}
