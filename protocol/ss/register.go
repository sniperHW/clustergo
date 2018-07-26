
package ss
import (
	"sanguo/codec/pb"
	"sanguo/protocol/ss/message"
	"sanguo/protocol/ss/rpc"
)

func init() {
	//普通消息
	pb.Register("ss",&message.Heartbeat{},1)
	pb.Register("ss",&message.Echo{},2)

	//rpc请求
	pb.Register("rpc_req",&rpc.EchoReq{},1)

	//rpc响应
	pb.Register("rpc_resp",&rpc.EchoResp{},1)

}
