package main

import (
	"fmt"
	"time"

	"github.com/sniperHW/clustergo/membership"
	"github.com/sniperHW/clustergo/membership/etcd"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func main() {

	membershipCli := etcd.Client{
		PrefixConfig: "/test/",
		PrefixAlive:  "/alive/",
		LogicAddr:    "1.1.1",
		TTL:          time.Second * 10,
		Cfg: clientv3.Config{
			Endpoints:   []string{"localhost:2379"},
			DialTimeout: time.Second * 5,
		},
	}

	membershipCli.Subscribe(func(di membership.MemberInfo) {
		fmt.Println("add", di.Add)
		fmt.Println("update", di.Update)
		fmt.Println("remove", di.Remove)
	})

	ch := make(chan struct{})

	<-ch
}
