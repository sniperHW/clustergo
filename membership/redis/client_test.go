package redis

import (
	"fmt"
	"testing"
	"time"

	"github.com/go-redis/redis"
	"github.com/sniperHW/clustergo/membership"
)

func TestGetMember(t *testing.T) {
	cli := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	cli.FlushAll()

	rcli := &Client{
		alive:    map[string]struct{}{},
		members:  map[string]*membership.Node{},
		RedisCli: cli,
	}

	if err := rcli.InitScript(); err != nil {
		panic(err)
	}

	err := rcli.UpdateMember(&Node{
		LogicAddr: "1.1.1",
		NetAddr:   "192.168.1.1:8011",
		Available: true,
	})
	if err != nil {
		panic(err)
	}

	err = rcli.UpdateMember(&Node{
		LogicAddr: "1.1.2",
		NetAddr:   "192.168.1.2:8011",
		Available: true,
	})
	if err != nil {
		panic(err)
	}

	rcli.getMembers()

	err = rcli.RemoveMember(&Node{
		LogicAddr: "1.1.2",
	})
	if err != nil {
		panic(err)
	}

	rcli.getMembers()

}

func TestGetAlive(t *testing.T) {
	cli := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	cli.FlushAll()

	{
		_, err := cli.Eval(ScriptHeartbeat, []string{"sniperHW1"}, 2).Result()
		fmt.Println("sniperHW1 heartbeat", GetRedisError(err))
	}

	{
		_, err := cli.Eval(ScriptHeartbeat, []string{"sniperHW2"}, 5).Result()
		fmt.Println("sniperHW2 heartbeat", GetRedisError(err))
	}

	rcli := &Client{
		alive:    map[string]struct{}{},
		members:  map[string]*membership.Node{},
		RedisCli: cli,
	}

	if err := rcli.InitScript(); err != nil {
		panic(err)
	}

	rcli.getAlives()

	c := make(chan struct{})

	go func() {
		m, err := cli.Subscribe("alive").ReceiveMessage()
		err = GetRedisError(err)
		fmt.Println("server version", m.Payload)
		if err == nil {
			_, err := cli.Eval(ScriptGetAlive, []string{}, 0).Result()
			err = GetRedisError(err)
			if err != nil {
				fmt.Println(err)
				return
			}
			//fmt.Println(re)
			//version := re.([]interface{})[0].(int64)
			//fmt.Println("alive version", version)
			//for _, v := range re.([]interface{})[1].([]interface{}) {
			//	fmt.Println(v.([]interface{})[0].(string), v.([]interface{})[1].(string))
			//}
			close(c)
		}
	}()

	time.Sleep(time.Second * 3)

	_, err := cli.Eval(ScriptCheckTimeout, []string{}).Result()
	err = GetRedisError(err)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("ScriptCheckTimeout")

	<-c

	rcli.getAlives()

	/*{
		re, err := cli.Eval(ScriptGetAlive, []string{}, 1).Result()
		err = GetRedisError(err)
		if err != nil {
			fmt.Println(err)
			return
		}
		version := re.([]interface{})[0].(int64)
		fmt.Println("alive version", version)
		for _, v := range re.([]interface{})[1].([]interface{}) {
			fmt.Println(v.([]interface{})[0].(string), v.([]interface{})[1].(string))
		}
	}*/

}
