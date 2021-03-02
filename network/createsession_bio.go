// +build !aio

package network

import (
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/socket"
	"net"
)

func CreateSession(conn net.Conn) kendynet.StreamSession {
	return socket.NewStreamSocket(conn)
}
