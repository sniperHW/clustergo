
package cs
import (
	"sanguo/codec/pb"
	"sanguo/protocol/cs/message"
)

func init() {
	//toS
	pb.Register("cs",&message.HeartbeatToS{},1)
	pb.Register("cs",&message.EchoToS{},2)

	//toC
	pb.Register("sc",&message.HeartbeatToC{},1)
	pb.Register("sc",&message.EchoToC{},2)

}
