package main

import(
	"sanguo/cs"
	"github.com/sniperHW/kendynet"
	cs_msg "sanguo/protocol/cs/message"
	codecs "sanguo/codec/cs"
	"fmt"
	"reflect"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet/util"
	"github.com/sniperHW/kendynet/golog"
	"sanguo/example/test/server/msg"
)

var (
	queue	*util.BlockQueue
)

type handler func(kendynet.StreamSession,*codecs.Message)

type dispatcher struct {
	handlers map[string]handler
}

func (this *dispatcher) Register(msg proto.Message,h handler) {
	msgName := reflect.TypeOf(msg).String()
	if nil == h {
		return
	}
	_,ok := this.handlers[msgName]
	if ok {
		return
	}

	this.handlers[msgName] = h
}

func (this *dispatcher) Dispatch(session kendynet.StreamSession,msg *codecs.Message) {
	if nil != msg {
		name := msg.GetName()
		handler,ok := this.handlers[name]
		if ok {
			queue.Add(func () {
				handler(session,msg)
			})
		}
	}
}

func (this *dispatcher) OnClose(session kendynet.StreamSession,reason string) {
	fmt.Printf("client close:%s\n",reason)
	player := session.GetUserData().(*msg.Player)
	if player != nil {
		queue.Add(func () {
			msg.RemovePlayerMap(player.Name)
		})

		queue.Add(func() {
			msg.SetAiPlayer()
		})
	}
}

func (this *dispatcher) OnNewClient(session kendynet.StreamSession) {
	fmt.Printf("new client\n")
}

func main() {
	outLogger := golog.NewOutputLogger("log","benmark_echo",1024*1024*50)
	kendynet.InitLogger(outLogger,"test-game")

	queue = util.NewBlockQueue()
	d := &dispatcher{handlers:make(map[string]handler)}
	d.Register(&cs_msg.EchoToS{},msg.OnEcho)
	d.Register(&cs_msg.LoginToS{},msg.OnLogin)
	d.Register(&cs_msg.AicontrolToS{},msg.OnAicontrol)
	d.Register(&cs_msg.ActormoveToS{},msg.OnActorMove)
	d.Register(&cs_msg.TakedamageToS{},msg.OnTakeDamage)
	d.Register(&cs_msg.ActorstateToS{},msg.OnActorState)

	err := cs.StartTcpServer("tcp","127.0.0.1:8010",d)

	go func() {
		for {
			_,localList := queue.Get()
			for _ , task := range localList {
				task.(func())()
			}
		}
	}()

	if nil != err {
		fmt.Printf("StartTcpServer failed:%s\n",err.Error())
	} else {
		fmt.Printf("server start on 127.0.0.1:8010\n")
		sigStop := make(chan bool)
		_,_ = <- sigStop
		fmt.Printf("server end\n")
	}

}