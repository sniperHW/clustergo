package redis

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sniperHW/clustergo/membership"
)

func TestClientSubscribe(t *testing.T) {
	cli := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	cli.FlushAll(context.Background())

	node1 := &MemberShip{
		RedisCli: cli,
	}

	if err := node1.Init(); err != nil {
		panic(err)
	}

	node1.Subscribe(func(di membership.MemberInfo) {
		fmt.Println("add", di.Add)
		fmt.Println("update", di.Update)
		fmt.Println("remove", di.Remove)
	})

	//time.Sleep(time.Second * 10)

	rcli := &MemberShip{
		RedisCli: redis.NewClient(&redis.Options{
			Addr: "localhost:6379",
		}),
	}

	if err := rcli.Init(); err != nil {
		panic(err)
	}

	fmt.Println("Update1")

	err := rcli.UpdateMember(&Node{
		LogicAddr: "1.1.1",
		NetAddr:   "192.168.1.1:8011",
		Available: true,
	})
	if err != nil {
		panic(err)
	}

	//time.Sleep(time.Second)

	fmt.Println("Update2")

	err = rcli.UpdateMember(&Node{
		LogicAddr: "1.1.2",
		NetAddr:   "192.168.1.2:8011",
		Available: true,
	})
	if err != nil {
		panic(err)
	}

	time.Sleep(time.Second * 2)

	err = rcli.RemoveMember(&Node{
		LogicAddr: "1.1.2",
	})
	if err != nil {
		panic(err)
	}

	time.Sleep(time.Second)

	err = rcli.UpdateMember(&Node{
		LogicAddr: "1.1.1",
		NetAddr:   "192.168.1.1:8012",
		Available: true,
	})
	if err != nil {
		panic(err)
	}

	time.Sleep(time.Second)

	fmt.Println("------------keepalive---------")

	rcli.KeepAlive(&Node{
		LogicAddr: "1.1.1",
	})

	time.Sleep(time.Second * 11)

	fmt.Println("ScriptCheckTimeout")

	rcli.CheckTimeout()

	time.Sleep(time.Second * 2)

}

func TestGetMember(t *testing.T) {
	cli := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	cli.FlushAll(context.Background())

	rcli := &MemberShip{
		RedisCli: cli,
	}

	if err := rcli.Init(); err != nil {
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
	cli.FlushAll(context.Background())

	{
		_, err := cli.Eval(context.Background(), ScriptHeartbeat, []string{"sniperHW1"}, 2).Result()
		fmt.Println("sniperHW1 heartbeat", GetRedisError(err))
	}

	{
		_, err := cli.Eval(context.Background(), ScriptHeartbeat, []string{"sniperHW2"}, 5).Result()
		fmt.Println("sniperHW2 heartbeat", GetRedisError(err))
	}

	rcli := &MemberShip{
		alive:    map[string]struct{}{},
		members:  map[string]*Node{},
		RedisCli: cli,
	}

	if err := rcli.Init(); err != nil {
		panic(err)
	}

	rcli.getAlives()

	c := make(chan struct{})

	go func() {
		m, err := cli.Subscribe(context.Background(), "alive").ReceiveMessage(context.Background())
		err = GetRedisError(err)
		fmt.Println("server version", m.Payload)
		if err == nil {
			_, err := cli.Eval(context.Background(), ScriptGetAlive, []string{}, 0).Result()
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

	_, err := cli.Eval(context.Background(), ScriptCheckTimeout, []string{}).Result()
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
