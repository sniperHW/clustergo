package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("app client|server")
		return
	}

	switch os.Args[1] {
	case "client":
		cli()
	case "server":
		svr()
	default:
		fmt.Println("app client|server")
	}
}
