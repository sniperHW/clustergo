package cs

import (
	codecs "github.com/sniperHW/sanguo/codec/cs"

	"github.com/sniperHW/kendynet"
)

type ServerDispatcher interface {
	Dispatch(kendynet.StreamSession, *codecs.Message)
	OnClose(kendynet.StreamSession, error)
	OnNewClient(kendynet.StreamSession)
	OnAuthenticate(kendynet.StreamSession) bool
}

type ClientDispatcher interface {
	Dispatch(kendynet.StreamSession, *codecs.Message)
	OnClose(kendynet.StreamSession, error)
	OnEstablish(kendynet.StreamSession)
	OnConnectFailed(peerAddr string, err error)
}
