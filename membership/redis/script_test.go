package redis

import (
	"fmt"
	"testing"
	"time"

	"github.com/go-redis/redis"
)

func TestMembers(t *testing.T) {
	cli := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	cli.FlushAll()

	{
		_, err := cli.Eval(ScriptUpdateMember, []string{"sniperHW1"}, "insert_update", "sniperHW's data").Result()
		fmt.Println(GetRedisError(err))
	}

	{
		_, err := cli.Eval(ScriptUpdateMember, []string{"sniperHW2"}, "insert_update", "sniperHW2's data").Result()
		fmt.Println(GetRedisError(err))
	}

	{
		_, err := cli.Eval(ScriptUpdateMember, []string{"sniperHW2"}, "delete").Result()
		fmt.Println(GetRedisError(err))
	}

	{
		re, err := cli.Eval(ScriptGetMembers, []string{}, 0).Result()
		if err != nil {
			fmt.Println(GetRedisError(err))
		}
		fmt.Println(re)
	}

	{
		re, err := cli.Eval(ScriptGetMembers, []string{}, 2).Result()
		if err != nil {
			fmt.Println(GetRedisError(err))
		}
		fmt.Println(re)
	}

	{
		re, err := cli.Eval(ScriptGetMembers, []string{}, 3).Result()
		if err != nil {
			fmt.Println(GetRedisError(err))
		}
		fmt.Println(re)
	}

}

func TestAlive(t *testing.T) {
	cli := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	cli.FlushAll()

	{
		_, err := cli.Eval(ScriptHeartbeat, []string{"sniperHW1"}, 2).Result()
		fmt.Println(GetRedisError(err))
	}

	{
		_, err := cli.Eval(ScriptHeartbeat, []string{"sniperHW2"}, 5).Result()
		fmt.Println(GetRedisError(err))
	}

	{
		re, err := cli.Eval(ScriptGetAlive, []string{}, 0).Result()
		if err != nil {
			fmt.Println(GetRedisError(err))
		}
		fmt.Println(re)
	}

	c := make(chan struct{})

	go func() {
		m, err := cli.Subscribe("alive").ReceiveMessage()
		fmt.Println(m, err)
		if err == nil {
			re, err := cli.Eval(ScriptGetAlive, []string{}, 0).Result()
			if err != nil {
				fmt.Println(GetRedisError(err))
			}
			fmt.Println(re)
			close(c)
		}
	}()

	time.Sleep(time.Second * 3)

	_, err := cli.Eval(ScriptCheckTimeout, []string{}).Result()
	if err != nil {
		fmt.Println(GetRedisError(err))
	}

	<-c
}
