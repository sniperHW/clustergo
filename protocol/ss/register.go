
package ss
import (
	"github.com/sniperHW/sanguo/codec/pb"
	"github.com/sniperHW/sanguo/protocol/ss/ssmessage"
	"github.com/sniperHW/sanguo/protocol/ss/rpc"
)

func init() {
	//普通消息
	pb.Register("ss",&ssmessage.Echo{},1001)
	pb.Register("ss",&ssmessage.KickGateUser{},1002)
	pb.Register("ss",&ssmessage.GateUserDestroy{},1003)
	pb.Register("ss",&ssmessage.SsToGate{},1004)
	pb.Register("ss",&ssmessage.SsToGateError{},1005)

	//rpc请求
	pb.Register("rpc_req",&rpc.EchoReq{},1001)
	pb.Register("rpc_req",&rpc.SynctokenReq{},1002)
	pb.Register("rpc_req",&rpc.GateUserLoginReq{},1003)
	pb.Register("rpc_req",&rpc.ForwardUserMsgReq{},1004)

	//rpc响应
	pb.Register("rpc_resp",&rpc.EchoResp{},1001)
	pb.Register("rpc_resp",&rpc.SynctokenResp{},1002)
	pb.Register("rpc_resp",&rpc.GateUserLoginResp{},1003)
	pb.Register("rpc_resp",&rpc.ForwardUserMsgResp{},1004)

}
