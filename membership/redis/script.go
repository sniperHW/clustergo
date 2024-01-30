package redis

// 更新一个member,add|modify|markdelete
const ScriptUpdateMember string = `
`

const ScriptGetMembers string = `
	redis.call('select',0)
	local clienrVer = tonumber(ARGV[1])
	local serverVer = redis.call('get','version')
	if not serverVer then
		return {clientVer}
	else
		serverVer = tonumber(serverVer)
	end

	--两端版本号一致，客户端的数据已经是最新的
	if clienrVer == serverVer then
		return {serverVer}
	end
	local nodes = {}
	local result = redis.call('scan',0)
	for k,v in pairs(result[2]) do
		if v ~= "version" then
			local node_version,net_addr,export,available,markdel = redis.call('hmget',v,'version','net_addr','export','available','markdel')
			node_version = tonumber(node_version)
			if cliVer == 0 then
				--初始状态,只返回markdel==false
				if not markdel or markdel == "false" then
					table.insert(nodes,{v,net_addr,export,available,"false"})
				end
			else if node_version > cliVer then
				--返回比客户端新的节点,包括markdel=="true"的节点，这样客户端可以在本地将这种节点删除	
				table.insert(nodes,{v,net_addr,export,available,markdel})
			end
		end
	end
	return {serverVer,nodes}
`

const ScriptHeartbeat string = `
	redis.call('select',1)
	local dead = redis.call('get','dead')
	local deadline = tonumber(redis.call('TIME')[1]) + 10 --10秒后超时

	if not dead or dead == "false" then
		local serverVer = redis.call('get','version')
		if not serverVer then
			serverVer = 1
		else
			serverVer = tonumber(serverVer) + 1
		end		
		redis.call('hmset','version',serverVer)
		redis.call('hmset',KEYS[1],'deadline',deadline,'version',serverVer,'dead','false')
		
		--publish
	else
		redis.call('hmset',KEYS[1],'deadline',deadline)
	end
`

const ScriptGetAlive string = `
	redis.call('select',1)
	local clienrVer = tonumber(ARGV[1])
	local serverVer = redis.call('get','version')
	if not serverVer then
		return {clienrVer}
	else
		serverVer = tonumber(serverVer)
	end

	--两端版本号一致，客户端的数据已经是最新的
	if clienrVer == serverVer then
		return {serverVer}
	end
	local nodes = {}
	local result = redis.call('scan',0)
	for k,v in pairs(result[2]) do
		if v ~= "version" then
			local node_version,dead = redis.call('hmget',v,'version','dead')
			node_version = tonumber(node_version)
			if cliVer == 0 then
				--初始状态,只返回dead==false
				if not dead or dead == "false" then
					table.insert(nodes,{v,net_addr,export,available,"false"})
				end
			else if node_version > cliVer then
				--返回比客户端新的节点,包括dead=="true"的节点，这样客户端可以在本地将这种节点删除	
				table.insert(nodes,{v,dead})
			end
		end
	end
	return {serverVer,nodes}
`

// 遍历db1,将超时节点标记为dead=true
const ScriptCheckTimeout string = `
`
