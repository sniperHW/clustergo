
package ss
import (
	"github.com/sniperHW/sanguo/codec/pb"
	"github.com/sniperHW/sanguo/protocol/ss/ssmessage"
	"github.com/sniperHW/sanguo/protocol/ss/rpc"
)

func init() {
	//普通消息
	pb.Register("ss",&ssmessage.Echo{},1001)

	//rpc请求
	pb.Register("rpc_req",&rpc.EchoReq{},1001)
	pb.Register("rpc_req",&rpc.GateToGameReq{},1002)

	//rpc响应
	pb.Register("rpc_resp",&rpc.EchoResp{},1001)
	pb.Register("rpc_resp",&rpc.GateToGameResp{},1002)

}
