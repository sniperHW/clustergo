package etcd

import (
	"time"

	"github.com/sniperHW/clustergo"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type Admin struct {
	Cfg          clientv3.Config
	PrefixConfig string
	PrefixAlive  string
	Logger       clustergo.Logger
	TTL          time.Duration

	//leaseID        clientv3.LeaseID
	//leaseCh        <-chan *clientv3.LeaseKeepAliveResponse
}
