package proc

import (
	"strings"
)

type Proc struct {
	User        string
	Pid         int
	CommandName string
	FullCommand string
	Cpu         float64
	Mem         float64
}

func createFilter(filter []string) func(string) bool {
	return func(s string) bool {
		if len(filter) == 0 {
			return true
		}
		for _, v := range filter {
			if strings.Contains(s, v) {
				return true
			}
		}
		return false
	}
}
