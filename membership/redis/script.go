package redis

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"io"
	"strings"

	"github.com/redis/go-redis/v9"
)

// 更新一个member,add|modify|markdelete
// KEYS[1]=addr, KEYS[2]=memberHashKey, KEYS[3]=memberVersionKey
// ARGV[1]=action("insert_update"/"delete"), ARGV[2]=data, ARGV[3]=channelName
const ScriptUpdateMember string = `
	local S = string.char(1)
	local serverVer = redis.call('get',KEYS[3])
	if not serverVer then
		serverVer = 1
	else
		serverVer = tonumber(serverVer) + 1
	end

	redis.call('set',KEYS[3],serverVer)
	if ARGV[1] == 'insert_update' then
		redis.call('hset',KEYS[2],KEYS[1],serverVer .. S .. 'false' .. S .. ARGV[2])
		redis.call('PUBLISH',ARGV[3],serverVer)
	elseif ARGV[1] == "delete" then
		local existing = redis.call('hget',KEYS[2],KEYS[1])
		if existing then
			local _, _, info = existing:match('^(.-)' .. S .. '(.-)' .. S .. '(.*)$')
			redis.call('hset',KEYS[2],KEYS[1],serverVer .. S .. 'true' .. S .. (info or ''))
			redis.call('PUBLISH',ARGV[3],serverVer)
		end
	end
`

// KEYS[1]=memberHashKey, KEYS[2]=memberVersionKey
// ARGV[1]=clientVersion
const ScriptGetMembers string = `
	local S = string.char(1)
	local clientVer = tonumber(ARGV[1])
	local serverVer = redis.call('get',KEYS[2])
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
	local all = redis.call('hgetall',KEYS[1])
	for i = 1, #all, 2 do
		local addr = all[i]
		local ver, markdel, info = all[i+1]:match('^(.-)' .. S .. '(.-)' .. S .. '(.*)$')
		if ver then
			if clientVer == 0 then
				if markdel == 'false' then
					table.insert(nodes,{addr,info,markdel})
				end
			elseif tonumber(ver) > clientVer then
				--返回比客户端新的节点,包括markdel=="true"的节点，这样客户端可以在本地将这种节点删除
				table.insert(nodes,{addr,info,markdel})
			end
		end
	end
	return {serverVer,nodes}
`

// KEYS[1]=addr, KEYS[2]=aliveHashKey, KEYS[3]=aliveVersionKey
// ARGV[1]=timeoutSeconds, ARGV[2]=channelName
const ScriptHeartbeat string = `
	local S = string.char(1)
	local nodeData = redis.call('hget',KEYS[2],KEYS[1])
	local deadline = tonumber(redis.call('TIME')[1]) + tonumber(ARGV[1])
	local prevDead = 'false'

	if nodeData then
		local _, d = nodeData:match('^(.-)' .. S .. '(.-)' .. S .. '.*$')
		if d then prevDead = d end
	end

	if prevDead == 'false' then
		local serverVer = redis.call('get',KEYS[3])
		if not serverVer then
			serverVer = 1
		else
			serverVer = tonumber(serverVer) + 1
		end
		redis.call('set',KEYS[3],serverVer)
		redis.call('hset',KEYS[2],KEYS[1],serverVer .. S .. 'false' .. S .. deadline)
		redis.call('PUBLISH',ARGV[2],serverVer)
	else
		redis.call('hset',KEYS[2],KEYS[1],nodeData:match('^(.-)' .. S .. '(.-)' .. S .. '.*$') .. S .. deadline)
	end
`

// KEYS[1]=aliveHashKey, KEYS[2]=aliveVersionKey
// ARGV[1]=clientVersion
const ScriptGetAlives string = `
	local S = string.char(1)
	local clientVer = tonumber(ARGV[1])
	local serverVer = redis.call('get',KEYS[2])
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
	local all = redis.call('hgetall',KEYS[1])
	for i = 1, #all, 2 do
		local addr = all[i]
		local ver, dead = all[i+1]:match('^(.-)' .. S .. '(.-)' .. S .. '.*$')
		if ver then
			if clientVer == 0 then
				--初始状态,只返回dead==false
				if dead == 'false' then
					table.insert(nodes,{addr,dead})
				end
			elseif tonumber(ver) > clientVer then
				--返回比客户端新的节点,包括dead=="true"的节点，这样客户端可以在本地将这种节点删除
				table.insert(nodes,{addr,dead})
			end
		end
	end
	return {serverVer,nodes}
`

// 遍历alive hash,将超时节点标记为dead=true
// KEYS[1]=aliveHashKey, KEYS[2]=aliveVersionKey
// ARGV[1]=channelName
const ScriptCheckTimeout string = `
	local S = string.char(1)
	local serverVer = redis.call('get',KEYS[2])
	if not serverVer then
		serverVer = 1
	else
		serverVer = tonumber(serverVer) + 1
	end

	local now = tonumber(redis.call('TIME')[1])
	local change = false
	local all = redis.call('hgetall',KEYS[1])

	for i = 1, #all, 2 do
		local addr = all[i]
		local _, dead, dl = all[i+1]:match('^(.-)' .. S .. '(.-)' .. S .. '(.+)$')
		if dead == 'false' and now > tonumber(dl) then
			if not change then
				change = true
				redis.call('set',KEYS[2],serverVer)
			end
			redis.call('hset',KEYS[1],addr,serverVer .. S .. 'true' .. S .. dl)
		end
	end

	if change then
		redis.call('PUBLISH',ARGV[1],serverVer)
	end
`

type script struct {
	src string
	sha string
}

func newScript(src string) *script {
	h := sha1.New()
	_, _ = io.WriteString(h, src)
	return &script{
		src: src,
		sha: hex.EncodeToString(h.Sum(nil)),
	}
}

func (s *script) eval(ctx context.Context, c *redis.Client, keys []string, args ...any) (result any, err error) {
	result, err = c.EvalSha(ctx, s.sha, keys, args...).Result()
	if err != nil && strings.Contains(err.Error(), "NOSCRIPT") {
		result, err = c.Eval(ctx, s.src, keys, args...).Result()
	}
	return
}

var (
	getMembers   = newScript(ScriptGetMembers)
	getAlives    = newScript(ScriptGetAlives)
	updateMember = newScript(ScriptUpdateMember)
	heartbeat    = newScript(ScriptHeartbeat)
	checkTimeout = newScript(ScriptCheckTimeout)
)
