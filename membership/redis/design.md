db0 存储配置
db1 存储 alive 信息

## db0

每个member使用logic.addr作为key。value中包含一个verison字段，version值为发生变更时db0.version。

单独的version字段，db0的变更通过lua脚本实现，每次变更version++，同时向notify member发布事件，所有监听事件的客户端重新
向db0获取数据，以更新本地信息。


## db1

记录live节点，所有活动节点定时执行heartbeat,heartbeat更新节点deadline。db1也有一个单独的version，每当向db1插入或删除节点,
version++。

每当节点插入db1,向notify live发布事件。所有监听事件的客户端重新向db1获取数据，以更新本地live信息。

一个单独的进程定时执行check_timeout脚本，check_timeout遍历db1中的节点，将超时节点删除(标记删除)，更新version,向notify live发布事件。













