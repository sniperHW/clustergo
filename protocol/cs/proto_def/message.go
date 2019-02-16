package proto_def

type st struct {
	Name      string
	Desc      string
	MessageID int
}

var CS_message = []st{
	st{"heartbeat", "心跳", 1},
	st{"echo", "测试用回射协议", 2},
	st{"gameLogin", "登陆游戏", 3},
	st{"reconnect", "重连", 4},
}
