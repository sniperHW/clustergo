package cluster

import (
	"github.com/sniperHW/sanguo/codec/pb"
)

func init() {
	pb.Register("ss", &Heartbeat{}, 1)

	pb.Register("rpc_req", &NotifyForginServicesH2SReq{}, 1)
	pb.Register("rpc_req", &AddForginServicesH2SReq{}, 2)
	pb.Register("rpc_req", &RemForginServicesH2SReq{}, 3)
	pb.Register("rpc_req", &NotifyForginServicesH2HReq{}, 4)
	pb.Register("rpc_req", &AddForginServicesH2HReq{}, 5)
	pb.Register("rpc_req", &RemForginServicesH2HReq{}, 6)

	//rpc响应
	pb.Register("rpc_resp", &NotifyForginServicesH2SResp{}, 1)
	pb.Register("rpc_resp", &AddForginServicesH2SResp{}, 2)
	pb.Register("rpc_resp", &RemForginServicesH2SResp{}, 3)
	pb.Register("rpc_resp", &NotifyForginServicesH2HResp{}, 4)
	pb.Register("rpc_resp", &AddForginServicesH2HResp{}, 5)
	pb.Register("rpc_resp", &RemForginServicesH2HResp{}, 6)

}
