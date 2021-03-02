package protocol

import (
	"github.com/sniperHW/sanguo/codec/pb"
)

const (
	CENTER_Login             uint16 = 1
	CENTER_LoginRet          uint16 = 2
	CENTER_HeartbeatToCenter uint16 = 3
	CENTER_HeartbeatToNode   uint16 = 4
	CENTER_NotifyNodeInfo    uint16 = 5
	CENTER_NotifyNodeAdd     uint16 = 6
	CENTER_NodeLeave         uint16 = 7
	CENTER_RemoveNode        uint16 = 8
)

func init() {
	pb.Register("center_req", &Login{}, 1)
	pb.Register("center_resp", &LoginRet{}, 2)
	pb.Register("center_msg", &HeartbeatToCenter{}, 3)
	pb.Register("center_msg", &HeartbeatToNode{}, 4)
	pb.Register("center_msg", &NotifyNodeInfo{}, 5)
	pb.Register("center_msg", &NodeAdd{}, 6)
	pb.Register("center_msg", &NodeLeave{}, 7)
	pb.Register("center_msg", &RemoveNode{}, 8)
}
