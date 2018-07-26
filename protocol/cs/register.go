
package cs
import (
	"sanguo/codec/pb"
	"sanguo/protocol/cs/message"
)

func init() {
	//toS
	pb.Register("cs",&message.HeartbeatToS{},1)
	pb.Register("cs",&message.EchoToS{},2)
	pb.Register("cs",&message.LoginToS{},3)
	pb.Register("cs",&message.ActormoveToS{},4)
	pb.Register("cs",&message.ActorstateToS{},5)
	pb.Register("cs",&message.TakedamageToS{},6)
	pb.Register("cs",&message.AicontrolToS{},7)
	pb.Register("cs",&message.AicontrolchangeToS{},8)

	//toC
	pb.Register("sc",&message.HeartbeatToC{},1)
	pb.Register("sc",&message.EchoToC{},2)
	pb.Register("sc",&message.LoginToC{},3)
	pb.Register("sc",&message.ActormoveToC{},4)
	pb.Register("sc",&message.ActorstateToC{},5)
	pb.Register("sc",&message.TakedamageToC{},6)
	pb.Register("sc",&message.AicontrolToC{},7)
	pb.Register("sc",&message.AicontrolchangeToC{},8)

}
