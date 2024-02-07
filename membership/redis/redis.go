package redis

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/go-redis/redis"
	"github.com/sniperHW/clustergo"
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
	memberVersion   atomic.Int64
	aliveVersion    atomic.Int64
	alive           map[string]struct{}         //健康节点
	members         map[string]*membership.Node //配置中的节点
	once            sync.Once
	LogicAddr       string
	Logger          clustergo.Logger
	cb              func(membership.MemberInfo)
	redisCli        *redis.Client
	getMembersSha   string
	heartbeatSha    string
	checkTimeoutSha string
	getAliveSha     string
	updateMemberSha string
}

func (cli *Client) initScriptSha() (err error) {
	if cli.getMembersSha, err = cli.redisCli.ScriptLoad(ScriptGetMembers).Result(); err != nil {
		err = fmt.Errorf("error on init ScriptGetMembers:%s", err.Error())
		return err
	}

	if cli.heartbeatSha, err = cli.redisCli.ScriptLoad(ScriptHeartbeat).Result(); err != nil {
		err = fmt.Errorf("error on init ScriptHeartbeat:%s", err.Error())
		return err
	}

	if cli.checkTimeoutSha, err = cli.redisCli.ScriptLoad(ScriptCheckTimeout).Result(); err != nil {
		err = fmt.Errorf("error on init ScriptHeartbeat:%s", err.Error())
		return err
	}

	if cli.getAliveSha, err = cli.redisCli.ScriptLoad(ScriptGetAlive).Result(); err != nil {
		err = fmt.Errorf("error on init ScriptHeartbeat:%s", err.Error())
		return err
	}

	if cli.updateMemberSha, err = cli.redisCli.ScriptLoad(ScriptUpdateMember).Result(); err != nil {
		err = fmt.Errorf("error on init ScriptHeartbeat:%s", err.Error())
		return err
	}

	return err
}

func (cli *Client) UpdateMember(n *Node) error {
	jsonBytes, _ := n.Marshal()
	_, err := cli.redisCli.EvalSha(cli.updateMemberSha, []string{n.LogicAddr}, "insert_update", string(jsonBytes)).Result()
	return GetRedisError(err)
}

func (cli *Client) RemoveMember(n *Node) error {
	_, err := cli.redisCli.EvalSha(cli.updateMemberSha, []string{n.LogicAddr}, "delete").Result()
	return GetRedisError(err)
}

func (cli *Client) getAlives() {
	/*re, err := cli.redisCli.EvalSha(cli.getAliveSha, []string{}, cli.aliveVersion.Load()).Result()
	err = GetRedisError(err)
	if err == nil {
		r := re.([]interface{})
		version := r[0].(int64)
		var alives []string
		if len(r) > 1 {
			for _, v := range r[1].([]interface{}) {
				alives = append(alives, v.(string))
			}
			sort.Slice(alives, func(i, j int) bool {
				return alives[i] < alives[j]
			})
		}

		var nodeinfo membership.MemberInfo
		cli.Lock()
		if version != cli.aliveVersion.Load() {
			cli.memberVersion.Store(version)
			var localAlives []string
			for k, _ := range cli.alive {
				localAlives = append(localAlives, k)
			}
			sort.Slice(localAlives, func(i, j int) bool {
				return localAlives[i] < localAlives[j]
			})

			i := 0
			j := 0
			for i < len(alives) && j < len(localAlives) {
				nodej := localAlives[j]
				nodei := alives[i]
				if nodei == nodej {
					i++
					j++
				} else if nodei > nodej {
					//移除节点
					delete(cli.alive, nodej)
					if n := cli.members[nodej]; n != nil {
						nn := *n
						nn.Available = false
						nodeinfo.Update = append(nodeinfo.Update, nn)
					}
					j++
				} else {
					//添加节点
					cli.alive[nodei] = struct{}{}
					if n := cli.members[nodei]; n != nil {
						if n.Available {
							nodeinfo.Update = append(nodeinfo.Update, *n)
						}
					}
					i++
				}
			}
			for _, v := range localAlives[j:] {
				delete(cli.alive, v)
				if n := cli.members[v]; n != nil {
					nn := *n
					nn.Available = false
					nodeinfo.Update = append(nodeinfo.Update, nn)
				}
			}

			for _, v := range alives[i:] {
				cli.alive[v] = struct{}{}
				if n := cli.members[v]; n != nil {
					if n.Available {
						nodeinfo.Update = append(nodeinfo.Update, *n)
					}
				}
			}
		}
		cli.Unlock()
	}*/
}

func (cli *Client) getMembers() {
	/*re, err := cli.redisCli.EvalSha(cli.getMembersSha, []string{}, cli.memberVersion.Load()).Result()
	err = GetRedisError(err)
	if err == nil {
		r := re.([]interface{})
		version := r[0].(int64)
		var members []membership.Node
		if len(r) > 1 {
			for _, v := range r[1].([]interface{}) {
				var n Node
				if err = n.Unmarshal([]byte(v.(string))); err == nil {
					nn := membership.Node{
						Export: n.Export,
					}
					nn.Addr, _ = addr.MakeAddr(n.LogicAddr, n.NetAddr)
					if _, ok := cli.alive[n.LogicAddr]; ok && n.Available {
						nn.Available = true
					}
					members = append(members, nn)
				}
			}
			sort.Slice(members, func(i, j int) bool {
				return members[i].Addr.LogicAddr() < members[j].Addr.LogicAddr()
			})
		}
		cli.Lock()
		var nodeinfo membership.MemberInfo
		if version != cli.memberVersion.Load() {
			cli.memberVersion.Store(version)
			var localMembers []membership.Node
			for _, v := range cli.members {
				localMembers = append(localMembers, *v)
			}
			sort.Slice(localMembers, func(i, j int) bool {
				return localMembers[i].Addr.LogicAddr().String() < localMembers[j].Addr.LogicAddr().String()
			})
			//比较members和localMembers的差异
			i := 0
			j := 0
			for i < len(members) && j < len(localMembers) {
				nodej := localMembers[j]
				nodei := members[i]
				if nodei.Addr.LogicAddr() == nodej.Addr.LogicAddr() {
					if nodei.Addr.NetAddr() != nodej.Addr.NetAddr() ||
						nodei.Available != nodej.Available {
						nodeinfo.Update = append(nodeinfo.Update, nodei)
					}
					i++
					j++
				} else if nodei.Addr.LogicAddr() > nodej.Addr.LogicAddr() {
					nodeinfo.Remove = append(nodeinfo.Remove, nodej)
					j++
				} else {
					nodeinfo.Add = append(nodeinfo.Add, nodei)
					i++
				}
			}
			nodeinfo.Add = append(nodeinfo.Add, members[i:]...)
			nodeinfo.Remove = append(nodeinfo.Remove, localMembers[j:]...)
		}
		cli.Unlock()

		if len(nodeinfo.Add) > 0 || len(nodeinfo.Remove) > 0 || len(nodeinfo.Update) > 0 {
			cli.cb(nodeinfo)
		}
	}*/
}
