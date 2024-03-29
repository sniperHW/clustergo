package main

import (
	"encoding/json"
	"os"

	"github.com/sniperHW/clustergo/example/membership"
)

func main() {
	var config []*membership.Node
	f, err := os.Open("./config.json")
	if err != nil {
		panic(err)
	}

	decoder := json.NewDecoder(f)
	err = decoder.Decode(&config)
	if err != nil {
		panic(err)
	}

	svr := membership.NewServer()

	svr.Start("127.0.0.1:18110", config)

}
