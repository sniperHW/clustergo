package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sniperHW/clustergo/membership"
)

func (cli *Membership) getAlives() error {
	re, err := getAlives.eval(context.Background(), cli.RedisCli,
		[]string{cli.getAliveKey(), cli.getAliveVersionKey()},
		cli.aliveVersion)
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

func (cli *Membership) getMembers() error {
	re, err := getMembers.eval(context.Background(), cli.RedisCli,
		[]string{cli.getMemberKey(), cli.getMemberVersionKey()},
		cli.memberVersion)
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
			if cli.Logger != nil {
				cli.Logger.Warnf("redis membership unmarshal member failed: %v", err)
			}
			continue
		} else if markdel == "true" {
			if _, ok := cli.members[n.Addr.LogicAddr().String()]; ok {
				if _, ok := cli.alive[n.Addr.LogicAddr().String()]; ok && n.Available {
					n.Available = true
				} else {
					n.Available = false
				}
				nodeinfo.Remove = append(nodeinfo.Remove, n)
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

func (cli *Membership) watch(ctx context.Context) {
	memberKey := cli.getMemberKey()
	aliveKey := cli.getAliveKey()
	pubsub := cli.RedisCli.Subscribe(ctx, memberKey, aliveKey)
	ch := pubsub.Channel()

	/*
	 *   如果更新Node的事件早于Subscribe,更新事件将丢失，因此必须设置超时时间，超时后尝试获取members和alives的更新
	 */

	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()

	hadError := false

	for {
		select {
		case m, ok := <-ch:
			if !ok {
				// channel已关闭（网络断开或Redis重启），重新订阅
				pubsub.Close()
				cli.resubscribe(ctx, &pubsub, &ch)
				hadError = true
				continue
			}
			switch m.Channel {
			case memberKey:
				if err := cli.getMembers(); err != nil {
					hadError = true
				}
			case aliveKey:
				if err := cli.getAlives(); err != nil {
					hadError = true
				}
			}
		case <-ticker.C:
			err1 := cli.getMembers()
			err2 := cli.getAlives()
			if err1 != nil || err2 != nil {
				hadError = true
			}
			if hadError && err1 == nil && err2 == nil {
				// 从错误中恢复，重置version强制全量同步以补回断线期间丢失的变更
				cli.memberVersion = 0
				cli.aliveVersion = 0
				cli.getMembers()
				cli.getAlives()
				hadError = false
			}
		case <-ctx.Done():
			pubsub.Close()
			return
		}
	}
}

func (cli *Membership) resubscribe(ctx context.Context, pubsub **redis.PubSub, ch *<-chan *redis.Message) {
	memberKey := cli.getMemberKey()
	aliveKey := cli.getAliveKey()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		p := cli.RedisCli.Subscribe(ctx, memberKey, aliveKey)
		// 验证连接是否真正可用
		if _, err := p.Receive(ctx); err != nil {
			p.Close()
			time.Sleep(time.Second * 2)
			continue
		}
		*pubsub = p
		*ch = p.Channel()
		return
	}
}

func (cli *Membership) close() {
	if cli.closed.CompareAndSwap(false, true) {
		if cli.closeFunc != nil {
			cli.closeFunc()
		}
	}
}

func (cli *Membership) Subscribe(cb func(membership.MemberInfo)) (func(), error) {

	once := false

	cli.once.Do(func() {
		once = true
		cli.alive = map[string]struct{}{}
		cli.members = map[string]*membership.Node{}
	})

	if once {
		cli.cb = cb

		err := cli.getMembers()
		if err != nil {
			return nil, err
		}
		err = cli.getAlives()
		if err != nil {
			return nil, err
		}

		ctx, cancel := context.WithCancel(context.Background())

		cli.closeFunc = cancel

		go cli.watch(ctx)
	}
	return cli.close, nil
}
