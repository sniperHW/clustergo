package proto_def

type st struct {
	Name      string
	Desc      string
	MessageID int
}

var CS_message = []st{

	st{"echo", "测试用回射协议", 1},
}
