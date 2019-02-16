package proto_def

type St struct {
	Name      string
	MessageID int
}

var SS_message = []St{
	//应用定义
	{"echo", 1},
	{"kickGateUser", 2},
	{"gateUserDestroy", 3},
	{"ssToGate", 4}, //内部服务到gate
	{"ssToGateError", 5},
}
