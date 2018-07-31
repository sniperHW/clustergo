
package ss
import (
	"sanguo/codec/pb"
	"sanguo/protocol/ss/message"
	"sanguo/protocol/ss/rpc"
)

func init() {
	//普通消息
	pb.Register("ss",&message.Echo{},1001)

	//rpc请求
	pb.Register("rpc_req",&rpc.EchoReq{},1001)

	//rpc响应
	pb.Register("rpc_resp",&rpc.EchoResp{},1001)

}
