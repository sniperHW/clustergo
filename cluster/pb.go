package cluster

import (
	"github.com/sniperHW/sanguo/codec/pb"
)

func init() {
	pb.Register("ss", &Heartbeat{}, 1)

	pb.Register("ss", &AddForginServicesH2S{}, 2)
	pb.Register("ss", &RemForginServicesH2S{}, 3)
	pb.Register("ss", &AddForginServicesH2H{}, 4)
	pb.Register("ss", &RemForginServicesH2H{}, 5)
}
