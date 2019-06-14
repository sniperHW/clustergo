package cs

import (
	codecs "github.com/sniperHW/sanguo/codec/cs"

	"github.com/sniperHW/kendynet"
)

type ServerDispatcher interface {
	Dispatch(kendynet.StreamSession, *codecs.Message)
	OnClose(kendynet.StreamSession, string)
	OnNewClient(kendynet.StreamSession)
}

type ClientDispatcher interface {
	Dispatch(kendynet.StreamSession, *codecs.Message)
	OnClose(kendynet.StreamSession, string)
	OnEstablish(kendynet.StreamSession)
	OnConnectFailed(peerAddr string, err error)
}
