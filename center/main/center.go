package main

import (
	"fmt"
	center "github.com/sniperHW/sanguo/center"
	"github.com/sniperHW/sanguo/util"
	"os"
)

func main() {
	filename := fmt.Sprintf("center_%s", os.Args[1])
	logger := util.NewLogger("log", filename, 1024*1024*50)
	center.Start(os.Args[1], logger)
}
