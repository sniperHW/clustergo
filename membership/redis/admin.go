package redis

import (
	"context"

	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/membership"
)

func (cli *Membership) UpdateMember(n membership.Node) error {
	jsonBytes, _ := n.Marshal()
	_, err := updateMember.eval(context.Background(), cli.RedisCli, []string{n.Addr.LogicAddr().String()}, "insert_update", string(jsonBytes))
	return GetRedisError(err)
}

func (cli *Membership) RemoveMember(n addr.LogicAddr) error {
	_, err := updateMember.eval(context.Background(), cli.RedisCli, []string{n.String()}, "delete")
	return GetRedisError(err)
}

func (cli *Membership) KeepAlive(n addr.LogicAddr, second int) error {
	_, err := heartbeat.eval(context.Background(), cli.RedisCli, []string{n.String()}, second)
	return GetRedisError(err)
}

func (cli *Membership) CheckTimeout() {
	checkTimeout.eval(context.Background(), cli.RedisCli, []string{})
}
