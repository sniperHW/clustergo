package redis

import (
	"context"

	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/membership"
)

func (cli *Membership) UpdateMember(n membership.Node) error {
	jsonBytes, _ := n.Marshal()
	_, err := updateMember.eval(context.Background(), cli.RedisCli,
		[]string{n.Addr.LogicAddr().String(), cli.getMemberKey(), cli.getMemberVersionKey()},
		"insert_update", string(jsonBytes), cli.getMemberKey())
	return GetRedisError(err)
}

func (cli *Membership) RemoveMember(n addr.LogicAddr) error {
	_, err := updateMember.eval(context.Background(), cli.RedisCli,
		[]string{n.String(), cli.getMemberKey(), cli.getMemberVersionKey()},
		"delete", "", cli.getMemberKey())
	return GetRedisError(err)
}

func (cli *Membership) KeepAlive(n addr.LogicAddr, second int) error {
	_, err := heartbeat.eval(context.Background(), cli.RedisCli,
		[]string{n.String(), cli.getAliveKey(), cli.getAliveVersionKey()},
		second, cli.getAliveKey())
	return GetRedisError(err)
}

func (cli *Membership) CheckTimeout() error {
	_, err := checkTimeout.eval(context.Background(), cli.RedisCli,
		[]string{cli.getAliveKey(), cli.getAliveVersionKey()},
		cli.getAliveKey())
	return GetRedisError(err)
}
