package addrDispose

import (
	"fmt"
	"strconv"
	"strings"
)

func AddrDispose(addr string) string {
	t := strings.Split(addr, ":")
	port, _ := strconv.Atoi(t[1])
	return fmt.Sprintf("0.0.0.0:%d", port)
}
