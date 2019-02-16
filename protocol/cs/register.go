
package cs
import (
	"github.com/sniperHW/sanguo/codec/pb"
	"github.com/sniperHW/sanguo/protocol/cs/message"
)

func init() {
	//toS
	pb.Register("cs",&message.HeartbeatToS{},1)
	pb.Register("cs",&message.EchoToS{},2)
	pb.Register("cs",&message.GameLoginToS{},3)
	pb.Register("cs",&message.ReconnectToS{},4)

	//toC
	pb.Register("sc",&message.HeartbeatToC{},1)
	pb.Register("sc",&message.EchoToC{},2)
	pb.Register("sc",&message.GameLoginToC{},3)
	pb.Register("sc",&message.ReconnectToC{},4)

}
