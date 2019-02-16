package protocol

import (
	"sanguo/codec/pb"
)

func init() {
	pb.Register("center_msg", &Login{}, 1)
	pb.Register("center_msg", &LoginFailed{}, 2)
	pb.Register("center_msg", &HeartbeatToCenter{}, 3)
	pb.Register("center_msg", &HeartbeatToNode{}, 4)
	pb.Register("center_msg", &NotifyNodeInfo{}, 5)
	pb.Register("center_msg", &NodeLeave{}, 6)
}
