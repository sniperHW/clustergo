package cluster

import (
	cluster_proto "github.com/sniperHW/sanguo/cluster/proto"
	"github.com/sniperHW/sanguo/codec/pb"
)

func init() {
	pb.Register("ss", &cluster_proto.Heartbeat{}, 1)

	pb.Register("rpc_req", &cluster_proto.NotifyForginServicesH2SReq{}, 1)
	pb.Register("rpc_req", &cluster_proto.AddForginServicesH2SReq{}, 2)
	pb.Register("rpc_req", &cluster_proto.RemForginServicesH2SReq{}, 3)
	pb.Register("rpc_req", &cluster_proto.NotifyForginServicesH2HReq{}, 4)
	pb.Register("rpc_req", &cluster_proto.AddForginServicesH2HReq{}, 5)
	pb.Register("rpc_req", &cluster_proto.RemForginServicesH2HReq{}, 6)

	//rpc响应
	pb.Register("rpc_resp", &cluster_proto.NotifyForginServicesH2SResp{}, 1)
	pb.Register("rpc_resp", &cluster_proto.AddForginServicesH2SResp{}, 2)
	pb.Register("rpc_resp", &cluster_proto.RemForginServicesH2SResp{}, 3)
	pb.Register("rpc_resp", &cluster_proto.NotifyForginServicesH2HResp{}, 4)
	pb.Register("rpc_resp", &cluster_proto.AddForginServicesH2HResp{}, 5)
	pb.Register("rpc_resp", &cluster_proto.RemForginServicesH2HResp{}, 6)

}
