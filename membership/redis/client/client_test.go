package client

//go test -run=Get
//go tool cover -html=coverage.out
/*
import (

	//"sync"
	"fmt"
	"testing"
	"time"

	//"time"
	"github.com/go-redis/redis"
)

func TestRedis(t *testing.T) {
	cli := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	r, err := cli.Eval(scriptHeartbeat, []string{"1.1.1"}, time.Now().Unix()).Result()
	fmt.Println(r, err)

	r, err = cli.Eval(scriptHeartbeat, []string{"1.1.2"}, time.Now().Unix()).Result()
	fmt.Println(r, err)

	r, err = cli.Eval(scriptHeartbeat, []string{"1.1.3"}, time.Now().Unix()).Result()
	fmt.Println(r, err)

	//r, err = cli.Eval(scriptGetAlive, []string{}, 0).Result()
	//fmt.Println(r, err)

	/*cli.HMSet("test", map[string]interface{}{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
		"key4": "value4",
	})

	cli.HMSet("fest", map[string]interface{}{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
		"key4": "value4",
	})

	const scriptScan string = `
		local result = redis.call('scan',0,'match','test')
		return result[2]
	`

	r, err := cli.Eval(scriptScan, []string{}).Result()

	fmt.Println(r, err)* /

	//c := &Client{
	//	redisCli: cli,
	//}
}*/
