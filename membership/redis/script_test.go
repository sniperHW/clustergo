package redis

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestMembers(t *testing.T) {
	cli := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	cli.FlushAll(context.Background())

	{
		_, err := cli.Eval(context.Background(), ScriptUpdateMember, []string{"sniperHW1"}, "insert_update", "sniperHW's data").Result()
		fmt.Println(GetRedisError(err))
	}

	{
		_, err := cli.Eval(context.Background(), ScriptUpdateMember, []string{"sniperHW2"}, "insert_update", "sniperHW2's data").Result()
		fmt.Println(GetRedisError(err))
	}

	{
		_, err := cli.Eval(context.Background(), ScriptUpdateMember, []string{"sniperHW2"}, "delete").Result()
		fmt.Println(GetRedisError(err))
	}

	{
		re, err := cli.Eval(context.Background(), ScriptGetMembers, []string{}, 0).Result()
		if err != nil {
			fmt.Println(GetRedisError(err))
		}
		fmt.Println(re)
	}

	{
		re, err := cli.Eval(context.Background(), ScriptGetMembers, []string{}, 2).Result()
		if err != nil {
			fmt.Println(GetRedisError(err))
		}
		fmt.Println(re)
	}

	{
		re, err := cli.Eval(context.Background(), ScriptGetMembers, []string{}, 3).Result()
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
	cli.FlushAll(context.Background())

	{
		_, err := cli.Eval(context.Background(), ScriptHeartbeat, []string{"sniperHW1"}, 2).Result()
		fmt.Println("sniperHW1 heartbeat", GetRedisError(err))
	}

	{
		_, err := cli.Eval(context.Background(), ScriptHeartbeat, []string{"sniperHW2"}, 5).Result()
		fmt.Println("sniperHW2 heartbeat", GetRedisError(err))
	}

	{
		re, err := cli.Eval(context.Background(), ScriptGetAlive, []string{}, 0).Result()
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
	}

	c := make(chan struct{})

	go func() {
		m, err := cli.Subscribe(context.Background(), "alive").ReceiveMessage(context.Background())
		err = GetRedisError(err)
		fmt.Println("server version", m.Payload)
		if err == nil {
			re, err := cli.Eval(context.Background(), ScriptGetAlive, []string{}, 0).Result()
			err = GetRedisError(err)
			if err != nil {
				fmt.Println(err)
				return
			}
			//fmt.Println(re)
			version := re.([]interface{})[0].(int64)
			fmt.Println("alive version", version)
			for _, v := range re.([]interface{})[1].([]interface{}) {
				fmt.Println(v.([]interface{})[0].(string), v.([]interface{})[1].(string))
			}
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

	{
		re, err := cli.Eval(context.Background(), ScriptGetAlive, []string{}, 1).Result()
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
	}

}

func TestRedis(t *testing.T) {
	cli := redis.NewClient(&redis.Options{
		Addr:       "localhost:6379",
		MaxRetries: 10,
	})
	cli.FlushAll(context.Background())

	cli.Set(context.Background(), "hello", "world", 0)

	v, err := cli.Get(context.Background(), "hello").Result()

	fmt.Println(v, err)

	time.Sleep(time.Second * 5)

	fmt.Println("again")

	v, err = cli.Get(context.Background(), "hello").Result()

	fmt.Println(v, err)

}

func TestRedisSubscribe(t *testing.T) {
	cli := redis.NewClient(&redis.Options{
		Addr:       "localhost:6379",
		MaxRetries: 10,
	})
	cli.FlushAll(context.Background())

	_, err := cli.Subscribe(context.Background(), "alive").ReceiveMessage(context.Background())
	err = GetRedisError(err)
	fmt.Println(err)

	time.Sleep(time.Second * 5)

	fmt.Println("again")

	_, err = cli.Subscribe(context.Background(), "alive").ReceiveMessage(context.Background())
	err = GetRedisError(err)
	fmt.Println(err)

}
