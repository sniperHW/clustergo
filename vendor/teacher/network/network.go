package network

import (
	"github.com/sniperHW/kendynet"
	_ "github.com/sniperHW/sanguo/protocol/cs" //触发pb注册
	"net"
	"time"
)

func Listen(nettype string, service string, onNewclient func(net.Conn)) (net.Listener, func(), error) {
	tcpAddr, err := net.ResolveTCPAddr(nettype, service)
	if nil != err {
		return nil, nil, err
	}
	listener, err := net.ListenTCP(nettype, tcpAddr)
	if nil != err {
		return nil, nil, err
	}

	serve := func() {
		for {
			conn, e := listener.Accept()
			if e != nil {
				if ne, ok := e.(net.Error); ok && ne.Temporary() {
					kendynet.GetLogger().Errorf("accept temp err: %v", ne)
					continue
				} else {
					return
				}
			} else {
				onNewclient(conn)
			}
		}
	}

	return listener, serve, nil

}

func Dial(nettype string, peerAddr string, timeout time.Duration) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: timeout}
	return dialer.Dial(nettype, peerAddr)
}
