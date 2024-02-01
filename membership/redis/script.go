package redis

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
