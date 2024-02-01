package redis

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/go-redis/redis"
	"github.com/sniperHW/clustergo"
	"github.com/sniperHW/clustergo/membership"
)

// 更新一个member,add|modify|markdelete
const ScriptUpdateMember string = `
	redis.call('select',0)
	local serverVer = redis.call('get','version')
	if not serverVer then
		serverVer = 1
	else
		serverVer = tonumber(serverVer) + 1
	end	

	redis.call('set','version',serverVer)
	if ARGV[1] == 'insert_update' then
		redis.call('hmset',KEYS[1],'version',serverVer,'info',ARGV[2],'markdel','false')
		--publish
		redis.call('PUBLISH',"members",serverVer)
	elseif ARGV[1] == "delete" then
		local v = redis.call('hget',KEYS[1],'version')
		if v then
			redis.call('hmset',KEYS[1],'version',serverVer,'markdel','true')
			--publish
			redis.call('PUBLISH',"members",serverVer)			
		end
	end
`

const ScriptGetMembers string = `
	redis.call('select',0)
	local clientVer = tonumber(ARGV[1])
	local serverVer = redis.call('get','version')
	if not serverVer then
		return {clientVer}
	else
		serverVer = tonumber(serverVer)
	end
	--两端版本号一致，客户端的数据已经是最新的
	if clientVer == serverVer then
		return {serverVer}
	end
	local nodes = {}
	local result = redis.call('scan',0)
	for k,v in pairs(result[2]) do
		if v ~= "version" then
			local r = redis.call('hmget',v,'version','info','markdel')
			local node_version,info,markdel = r[1],r[2],r[3]
			if clientVer == 0 then
				if markdel == "false" then
					table.insert(nodes,{v,info,markdel})
				end
			elseif tonumber(node_version) > clientVer then
				--返回比客户端新的节点,包括markdel=="true"的节点，这样客户端可以在本地将这种节点删除
				table.insert(nodes,{v,info,markdel})
			end
		end
	end
	return {serverVer,nodes}
`

const ScriptHeartbeat string = `
	redis.call('select',1)
	local dead = redis.call('get','dead')
	local deadline = tonumber(redis.call('TIME')[1]) + tonumber(ARGV[1])

	if not dead or dead == "false" then
		local serverVer = redis.call('get','version')
		if not serverVer then
			serverVer = 1
		else
			serverVer = tonumber(serverVer) + 1
		end		
		redis.call('set','version',serverVer)
		redis.call('hmset',KEYS[1],'deadline',deadline,'version',serverVer,'dead','false')
		--publish
		redis.call('PUBLISH',"alive",serverVer)
	else
		redis.call('hmset',KEYS[1],'deadline',deadline)
	end
`

const ScriptGetAlive string = `
	redis.call('select',1)
	local clientVer = tonumber(ARGV[1])
	local serverVer = redis.call('get','version')
	if not serverVer then
		return {clientVer}
	else
		serverVer = tonumber(serverVer)
	end

	--两端版本号一致，客户端的数据已经是最新的
	if clientVer == serverVer then
		return {serverVer}
	end
	local nodes = {}
	local result = redis.call('scan',0)
	for k,v in pairs(result[2]) do
		if v ~= "version" then
			local r = redis.call('hmget',v,'version','dead')
			local node_version,dead = r[1],r[2]
			if clientVer == 0 then
				--初始状态,只返回dead==false
				if dead == "false" then
					table.insert(nodes,{v,dead})
				end
			elseif tonumber(node_version) > clientVer then
				--返回比客户端新的节点,包括dead=="true"的节点，这样客户端可以在本地将这种节点删除	
				table.insert(nodes,{v,dead})
			end
		end
	end
	return {serverVer,nodes}
`

// 遍历db1,将超时节点标记为dead=true
const ScriptCheckTimeout string = `
	redis.call('select',1)
	local serverVer = redis.call('get','version')
	if not serverVer then
		serverVer = 1
	else
		serverVer = tonumber(serverVer) + 1
	end

	local now = tonumber(redis.call('TIME')[1])

	local change = false	
	local result = redis.call('scan',0)
	for k,v in pairs(result[2]) do
		if v ~= "version" then
			local r = redis.call('hmget',v,'dead','deadline')
			local dead,deadline = r[1],r[2]
			if dead == "false" and now > tonumber(deadline) then
				if change == false then
					change = true
					redis.call('set','version',serverVer)
				end
				redis.call('hmset',v,'version',serverVer,'dead','true')
			end
		end
	end

	if change then
		--publish
		redis.call('PUBLISH','alive',serverVer)
	end
`

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
	version         atomic.Int64
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

func (cli *Client) getMembers() {
	re, err := cli.redisCli.EvalSha(cli.getMembersSha, []string{}, cli.version.Load()).Result()
	err = GetRedisError(err)
	if err == nil {
		r := re.([]interface{})
		version := r[0].(int64)
		var members []*Node
		if len(r) > 1 {
			for _, v := range r[1].([]interface{}) {
				var n Node
				if err = n.Unmarshal([]byte(v.(string))); err == nil {
					members = append(members, &n)
				}
			}
			sort.Slice(members, func(i, j int) bool {
				return members[i].LogicAddr < members[j].LogicAddr
			})
		}
		cli.Lock()
		var nodeinfo membership.MemberInfo
		if version != cli.version.Load() {
			cli.version.Store(version)
			var localMembers []*membership.Node
			for _, v := range cli.members {
				localMembers = append(localMembers, v)
			}
			sort.Slice(localMembers, func(i, j int) bool {
				return localMembers[i].Addr.LogicAddr().String() < localMembers[j].Addr.LogicAddr().String()
			})
			//比较members和localMembers的差异
		}
		cli.Unlock()

		if len(nodeinfo.Add) > 0 || len(nodeinfo.Remove) > 0 || len(nodeinfo.Update) > 0 {
			cli.cb(nodeinfo)
		}
	}
}
