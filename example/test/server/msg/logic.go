package msg

import(
	"github.com/sniperHW/kendynet"
	cs_msg "sanguo/protocol/cs/message"
	codecs "sanguo/codec/cs"
	"fmt"
	"github.com/golang/protobuf/proto"
)

var aiPlayer string

func SetAiPlayer() {
	if aiPlayer != "" {
		size := GetPlayerMapSize()
		if size == 0 {
			aiPlayer = ""
		}else {

			nodePlayer := GetPlayerFirstNode()
			if nodePlayer == nil {
				panic("aiPlayer is null")
			}
			aiPlayer = *nodePlayer
			ForeachPlayerAiControlChange(aiPlayer)
		}
	}
}

func CheckLogin(session kendynet.StreamSession) *string {
	userData := session.GetUserData()
	if userData == nil {
		fmt.Println("player not login")
		return nil
	}

	player := session.GetUserData().(*Player)
	if player == nil {
		panic("player is null")
	}
	return &player.Name
}

func OnEcho(session kendynet.StreamSession,msg *codecs.Message) {
	EchoToS := msg.GetData().(*cs_msg.EchoToS)
	fmt.Printf("on EchoToS %d:%s\n",msg.GetSeriNo(),EchoToS.GetMsg())
	EchoToC := &cs_msg.EchoToC{}
	EchoToC.Msg = proto.String("world")
	session.Send(codecs.NewMessage(msg.GetSeriNo(),EchoToC))
}

func OnLogin(session kendynet.StreamSession,msg *codecs.Message) {
	LoginToS := msg.GetData().(*cs_msg.LoginToS)
	fmt.Printf("on LoginToS name :%s\n",LoginToS.GetName())

	size := GetPlayerMapSize()
	if size == 0 && aiPlayer == "" {
		aiPlayer = LoginToS.GetName()
	}

	fmt.Printf("ai name: %s\n",aiPlayer)
	fmt.Println("test 1")
	var code cs_msg.EnumType
	LoginToC := &cs_msg.LoginToC{}

	userData := session.GetUserData()
	if userData != nil {
		player := session.GetUserData().(*Player)
		if player != nil {
			player = FindPLayerMap(player.Name)
			if player != nil {
				code = cs_msg.EnumType_LOGIN_ERROR
				LoginToC.Code = &code
				session.Send(codecs.NewMessage(msg.GetSeriNo(), LoginToC))
				fmt.Println("test 2")
			}else{
				code = cs_msg.EnumType_ERROR
				LoginToC.Code = &code
				fmt.Println("player name not found error")
			}
		}else {
			code = cs_msg.EnumType_ERROR
			LoginToC.Code = &code
			fmt.Println("user data nil error")
		}
	}else {
		player := FindPLayerMap(LoginToS.GetName())
		if player != nil {
			fmt.Println("repeat login ")
			player.Session.Close("repeat login error", 0)
			player.Session = session
			code = cs_msg.EnumType_OK
			LoginToC.Code = &code
			session.Send(codecs.NewMessage(msg.GetSeriNo(),LoginToC))
			fmt.Println("test 3")
		} else {
			player := Player{}
			player.Name = LoginToS.GetName()
			player.Session = session
			session.SetUserData(&player)
			AddPlayerMap(player.Name,player)
			code = cs_msg.EnumType_OK
			LoginToC.Code = &code
			session.Send(codecs.NewMessage(msg.GetSeriNo(),LoginToC))
			fmt.Println("test 4")
		}
		ForeachPlayerAiControlChange(aiPlayer)
	}
}

func OnAicontrol(session kendynet.StreamSession,msg *codecs.Message) {
	AicontrolToS := msg.GetData().(*cs_msg.AicontrolToS)
	fmt.Printf("on AicontrolToS %d:%s\n",msg.GetSeriNo(),AicontrolToS.GetStateName())

	name := CheckLogin(session)
	if name != nil {
		ForeachPlayerAiControl(AicontrolToS.GetStateName())
	}
}

func OnActorMove(session kendynet.StreamSession,msg *codecs.Message) {
	ActormoveToS := msg.GetData().(*cs_msg.ActormoveToS)
	fmt.Printf("on AicontrolToS statename %d\n",msg.GetSeriNo())

	name := CheckLogin(session)
	if name != nil {
		ForeachPlayerActorMove(*name,ActormoveToS.CurrentPosition,ActormoveToS.Speed)
	}
}

func OnTakeDamage(session kendynet.StreamSession,msg *codecs.Message) {
	TakedamageToS := msg.GetData().(*cs_msg.TakedamageToS)
	fmt.Printf("on TakeDamageToS %d %d\n",msg.GetSeriNo(),TakedamageToS.GetDamagePoint())

	name := CheckLogin(session)
	if name != nil {
		ForeachPlayerTakeDamage(*name,TakedamageToS.GetDamagePoint())
	}
}

func OnActorState(session kendynet.StreamSession,msg *codecs.Message) {
	ActorstateToS := msg.GetData().(*cs_msg.ActorstateToS)
	fmt.Printf("on ActorState %d %s\n",msg.GetSeriNo(),ActorstateToS.GetStateName())

	name := CheckLogin(session)
	if name != nil {
		ForeachPlayerActorState(*name,ActorstateToS.GetStateName())
	}
}