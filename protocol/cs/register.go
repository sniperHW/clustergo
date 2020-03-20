
package cs
import (
	"github.com/sniperHW/sanguo/codec/pb"
	"github.com/sniperHW/sanguo/protocol/cs/message"
)

func init() {
	//toS
	pb.Register("cs",&message.EchoToS{},1)

	//toC
	pb.Register("sc",&message.EchoToC{},1)

}
