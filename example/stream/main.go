package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("app client|game|gate")
		return
	}

	switch os.Args[1] {
	case "client":
		clientmain()
	case "gate":
		gatemain()
	case "game":
		gamemain()
	default:
		fmt.Println("app client|game|gate")
	}

}
