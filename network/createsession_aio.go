// +build aio

package network

import (
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/socket/aio"
	codecs "github.com/sniperHW/sanguo/codec/cs"
	"net"
)

var aioService *aio.SocketService = aio.NewSocketService(codecs.GetBufferPool())

func CreateSession(conn net.Conn) kendynet.StreamSession {
	return aio.NewSocket(aioService, conn)
}
