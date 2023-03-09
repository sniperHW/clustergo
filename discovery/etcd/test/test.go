package main

import (
	"fmt"
	"time"

	"github.com/sniperHW/clustergo/discovery"
	"github.com/sniperHW/clustergo/discovery/etcd"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func main() {

	discoveryCli := etcd.Discovery{
		Prefix: "/test/",
		Cfg: clientv3.Config{
			Endpoints:   []string{"localhost:2379"},
			DialTimeout: time.Second * 5,
		},
	}

	discoveryCli.Subscribe(func(di discovery.DiscoveryInfo) {
		fmt.Println(di)
	})

	ch := make(chan struct{})

	<-ch
}
