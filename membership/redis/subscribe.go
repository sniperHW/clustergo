package redis

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/membership"
)

type Subscribe struct {
	RedisCli      *redis.Client
	memberVersion int64
	aliveVersion  int64
	alive         map[string]struct{}         //健康节点
	members       map[string]*membership.Node //*membership.Node //配置中的节点
	cb            func(membership.MemberInfo)
	getMembersSha string
	getAliveSha   string
	once          sync.Once
	closeFunc     context.CancelFunc
	closed        atomic.Bool
}

func (cli *Subscribe) Init() (err error) {
	cli.alive = map[string]struct{}{}
	cli.members = map[string]*membership.Node{}
	if cli.getMembersSha, err = cli.RedisCli.ScriptLoad(context.Background(), ScriptGetMembers).Result(); err != nil {
		err = fmt.Errorf("error on init ScriptGetMembers:%s", err.Error())
		return err
	}

	if cli.getAliveSha, err = cli.RedisCli.ScriptLoad(context.Background(), ScriptGetAlive).Result(); err != nil {
		err = fmt.Errorf("error on init getAlive:%s", err.Error())
		return err
	}

	return err
}

func makeaddr(logicAddr, netAddr string) addr.Addr {
	a, _ := addr.MakeAddr(logicAddr, netAddr)
	return a
}

func (cli *Subscribe) getAlives() error {
	re, err := cli.RedisCli.EvalSha(context.Background(), cli.getAliveSha, []string{}, cli.aliveVersion).Result()
	if err = GetRedisError(err); err != nil {
		return err
	}

	r := re.([]interface{})
	version := r[0].(int64)
	if version == cli.aliveVersion {
		return nil
	}
	cli.aliveVersion = version
	var nodeinfo membership.MemberInfo
	for _, v := range r[1].([]interface{}) {
		addr, dead := v.([]interface{})[0].(string), v.([]interface{})[1].(string)
		if dead == "true" {
			delete(cli.alive, addr)
			if n, ok := cli.members[addr]; ok {
				//标记为不可用状态
				nodeinfo.Update = append(nodeinfo.Update, membership.Node{
					Addr:      n.Addr,
					Export:    n.Export,
					Available: false,
				})
			}
		} else if _, ok := cli.alive[addr]; !ok {
			cli.alive[addr] = struct{}{}
			if n, ok := cli.members[addr]; ok && n.Available {
				nodeinfo.Update = append(nodeinfo.Update, membership.Node{
					Addr:      n.Addr,
					Export:    n.Export,
					Available: true,
				})
			}
		}
	}
	if cli.cb != nil && len(nodeinfo.Update) > 0 {
		cli.cb(nodeinfo)
	}
	return nil
}

func (cli *Subscribe) getMembers() error {
	re, err := cli.RedisCli.EvalSha(context.Background(), cli.getMembersSha, []string{}, cli.memberVersion).Result()
	if err = GetRedisError(err); err != nil {
		return err
	}

	r := re.([]interface{})
	version := r[0].(int64)
	if version == cli.memberVersion {
		return nil
	}
	cli.memberVersion = version
	var nodeinfo membership.MemberInfo
	for _, v := range r[1].([]interface{}) {
		m, markdel := v.([]interface{})[1].(string), v.([]interface{})[2].(string)
		var n membership.Node
		if err = n.Unmarshal([]byte(m)); err != nil {
			continue
		} else if markdel == "true" {
			if nn, ok := cli.members[n.Addr.LogicAddr().String()]; ok {
				nodeinfo.Remove = append(nodeinfo.Remove, *nn)
				delete(cli.members, n.Addr.LogicAddr().String())
			}
		} else {
			logicAddr := n.Addr.LogicAddr().String()

			nn := membership.Node{
				Addr:   n.Addr,
				Export: n.Export,
			}

			if _, ok := cli.alive[logicAddr]; ok && n.Available {
				nn.Available = true
			}

			if _, ok := cli.members[logicAddr]; ok {
				nodeinfo.Update = append(nodeinfo.Update, nn)
			} else {
				nodeinfo.Add = append(nodeinfo.Add, nn)
			}
			cli.members[logicAddr] = &n

		}
	}
	if cli.cb != nil && (len(nodeinfo.Add) > 0 || len(nodeinfo.Update) > 0 || len(nodeinfo.Remove) > 0) {
		cli.cb(nodeinfo)
	}
	return nil
}

func (cli *Subscribe) watch(ctx context.Context) {
	ch := cli.RedisCli.Subscribe(ctx, "members", "alive").Channel()

	/*
	 *   如果更新Node的事件早于Subscribe,更新事件将丢失，因此必须设置超时时间，超时后尝试获取members和alives的更新
	 */

	ticker := time.NewTicker(time.Second * 5)
	for {
		select {
		case m := <-ch:
			switch m.Channel {
			case "members":
				cli.getMembers()
			case "alive":
				cli.getAlives()
			}
		case <-ticker.C:
			cli.getMembers()
			cli.getAlives()
		case <-ctx.Done():
			return
		}
	}
}

func (cli *Subscribe) Close() {
	if cli.closed.CompareAndSwap(false, true) {
		if cli.closeFunc != nil {
			cli.closeFunc()
		}
	}
}

func (cli *Subscribe) Subscribe(cb func(membership.MemberInfo)) error {

	once := false

	cli.once.Do(func() {
		once = true
	})

	if once {
		cli.cb = cb

		err := cli.getMembers()
		if err != nil {
			return err
		}
		err = cli.getAlives()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithCancel(context.Background())

		cli.closeFunc = cancel

		go cli.watch(ctx)
	}
	return nil
}
