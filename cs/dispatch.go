package cs

import(
	"github.com/sniperHW/kendynet"
	codecs "sanguo/codec/cs"
)

type ServerDispatcher interface {
	Dispatch(kendynet.StreamSession,*codecs.Message)
	OnClose(kendynet.StreamSession,string)
	OnNewClient(kendynet.StreamSession)
}


type ClientDispatcher interface {
	Dispatch(kendynet.StreamSession,*codecs.Message)
	OnClose(kendynet.StreamSession,string)
	OnEstablish(kendynet.StreamSession)
	OnConnectFailed(peerAddr string,err error)
}