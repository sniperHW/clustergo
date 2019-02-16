package ss

import (
	codess "sanguo/codec/ss"

	"github.com/sniperHW/kendynet"
)

type ServerDispatcher interface {
	Dispatch(kendynet.StreamSession, *codess.Message)
	OnClose(kendynet.StreamSession, string)
	OnNewClient(kendynet.StreamSession)
}

type ClientDispatcher interface {
	Dispatch(kendynet.StreamSession, *codess.Message)
	OnClose(kendynet.StreamSession, string)
	OnEstablish(kendynet.StreamSession)
	OnConnectFailed(peerAddr string, err error)
}
