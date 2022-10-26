package main

import (
	"encoding/json"
	"os"

	"github.com/sniperHW/sanguo/example/discovery"
)

func main() {
	var config []*discovery.Node
	f, err := os.Open("./config.json")
	if err != nil {
		panic(err)
	}

	decoder := json.NewDecoder(f)
	err = decoder.Decode(&config)
	if err != nil {
		panic(err)
	}

	svr := discovery.NewServer()

	svr.Start("127.0.0.1:8110", config)

}
