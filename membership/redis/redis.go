package redis

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis"
	"github.com/sniperHW/clustergo"
	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/membership"
)

func GetRedisError(err error) error {
	if err == nil || err.Error() == "redis: nil" {
		return nil
	} else {
		return err
	}
}

type Node struct {
	LogicAddr string `json:""`
	NetAddr   string `json:"netAddr"`
	Export    bool   `json:"export"`
	Available bool   `json:"available"`
}

func (n *Node) Marshal() ([]byte, error) {
	return json.Marshal(n)
}

func (n *Node) Unmarshal(data []byte) error {
	return json.Unmarshal(data, n)
}

type Client struct {
	sync.Mutex
	LogicAddr       string
	Logger          clustergo.Logger
	RedisCli        *redis.Client
	memberVersion   atomic.Int64
	aliveVersion    atomic.Int64
	alive           map[string]struct{}         //健康节点
	members         map[string]*membership.Node //配置中的节点
	cb              func(membership.MemberInfo)
	getMembersSha   string
	heartbeatSha    string
	checkTimeoutSha string
	getAliveSha     string
	updateMemberSha string
	once            sync.Once
	closed          atomic.Bool
}

func (cli *Client) InitScript() (err error) {
	if cli.getMembersSha, err = cli.RedisCli.ScriptLoad(ScriptGetMembers).Result(); err != nil {
		err = fmt.Errorf("error on init ScriptGetMembers:%s", err.Error())
		return err
	}

	if cli.heartbeatSha, err = cli.RedisCli.ScriptLoad(ScriptHeartbeat).Result(); err != nil {
		err = fmt.Errorf("error on init ScriptHeartbeat:%s", err.Error())
		return err
	}

	if cli.checkTimeoutSha, err = cli.RedisCli.ScriptLoad(ScriptCheckTimeout).Result(); err != nil {
		err = fmt.Errorf("error on init checkTimeout:%s", err.Error())
		return err
	}

	if cli.getAliveSha, err = cli.RedisCli.ScriptLoad(ScriptGetAlive).Result(); err != nil {
		err = fmt.Errorf("error on init getAlive:%s", err.Error())
		return err
	}

	if cli.updateMemberSha, err = cli.RedisCli.ScriptLoad(ScriptUpdateMember).Result(); err != nil {
		err = fmt.Errorf("error on init updateMember:%s", err.Error())
		return err
	}

	return err
}

func (cli *Client) UpdateMember(n *Node) error {
	jsonBytes, _ := n.Marshal()
	_, err := cli.RedisCli.EvalSha(cli.updateMemberSha, []string{n.LogicAddr}, "insert_update", string(jsonBytes)).Result()
	return GetRedisError(err)
}

func (cli *Client) RemoveMember(n *Node) error {
	_, err := cli.RedisCli.EvalSha(cli.updateMemberSha, []string{n.LogicAddr}, "delete").Result()
	return GetRedisError(err)
}

func (cli *Client) getAlives() error {
	re, err := cli.RedisCli.EvalSha(cli.getAliveSha, []string{}, cli.aliveVersion.Load()).Result()
	err = GetRedisError(err)
	if err == nil {
		r := re.([]interface{})
		version := r[0].(int64)
		var nodeinfo membership.MemberInfo
		cli.Lock()
		if version != cli.aliveVersion.Load() {
			cli.aliveVersion.Store(version)
			for _, v := range r[1].([]interface{}) {
				addr, dead := v.([]interface{})[0].(string), v.([]interface{})[1].(string)
				if dead == "true" {
					delete(cli.alive, addr)
					if n, ok := cli.members[addr]; ok {
						//标记为不可用状态
						nn := *n
						nn.Available = false
						nodeinfo.Update = append(nodeinfo.Update, nn)
					}
				} else {
					if _, ok := cli.alive[addr]; !ok {
						cli.alive[addr] = struct{}{}
						if n, ok := cli.members[addr]; ok && n.Available {
							nodeinfo.Update = append(nodeinfo.Update, *n)
						}
					}
				}
			}
		}
		fmt.Println(cli.aliveVersion.Load(), cli.alive)
		cli.Unlock()
		if cli.cb != nil && len(nodeinfo.Update) > 0 {
			cli.cb(nodeinfo)
		}
	}
	return err
}

func (cli *Client) getMembers() error {
	re, err := cli.RedisCli.EvalSha(cli.getMembersSha, []string{}, cli.memberVersion.Load()).Result()
	err = GetRedisError(err)
	if err == nil {
		r := re.([]interface{})
		version := r[0].(int64)
		var nodeinfo membership.MemberInfo
		cli.Lock()
		if version != cli.memberVersion.Load() {
			cli.memberVersion.Store(version)
			for _, v := range r[1].([]interface{}) {
				m, markdel := v.([]interface{})[1].(string), v.([]interface{})[2].(string)
				var n Node
				if err = n.Unmarshal([]byte(m)); err == nil {
					if markdel == "true" {
						if nn, ok := cli.members[n.LogicAddr]; ok {
							delete(cli.members, n.LogicAddr)
							nodeinfo.Remove = append(nodeinfo.Remove, *nn)
						}
					} else {
						if address, err := addr.MakeAddr(n.LogicAddr, n.NetAddr); err == nil {
							nn := membership.Node{
								Addr:   address,
								Export: n.Export,
							}
							if _, ok := cli.alive[n.LogicAddr]; ok && n.Available {
								nn.Available = true
							}

							if _, ok := cli.members[n.LogicAddr]; ok {
								nodeinfo.Update = append(nodeinfo.Update, nn)
							} else {
								nodeinfo.Add = append(nodeinfo.Add, nn)
							}
							cli.members[n.LogicAddr] = &nn
						}
					}
				}
			}
		}
		fmt.Println(cli.memberVersion.Load())
		for _, v := range cli.members {
			fmt.Println(v.Addr.LogicAddr().String(), v.Addr.NetAddr().String(), v.Available, v.Export)
		}
		cli.Unlock()

		if cli.cb != nil && (len(nodeinfo.Add) > 0 || len(nodeinfo.Update) > 0 || len(nodeinfo.Remove) > 0) {
			cli.cb(nodeinfo)
		}

	}
	return err
}

func (cli *Client) watch() {

	recover := func() {
		for {
			err1 := cli.getMembers()
			err2 := cli.getAlives()
			if err1 == nil && err2 == nil {
				return
			} else {
				time.Sleep(time.Second)
			}
		}
	}

	for {
		m, err := cli.RedisCli.Subscribe("members", "alive").ReceiveMessage()
		if err != nil {
			recover()
		} else {
			switch m.Channel {
			case "members":
				err = cli.getMembers()
			case "alive":
				err = cli.getAlives()
			}
			if err != nil {
				recover()
			}
		}
	}
}

func (cli *Client) Subscribe(cb func(membership.MemberInfo)) error {

	once := false

	cli.once.Do(func() {
		once = true
	})

	if once {
		if cli.closed.Load() {
			return errors.New("closed")
		}
		cli.cb = cb
		err := cli.getMembers()
		if err != nil {
			return err
		}
		err = cli.getAlives()
		if err != nil {
			return err
		}
		go cli.watch()
	}
	return nil
}
