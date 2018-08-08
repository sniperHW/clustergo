package msg

import (
	"github.com/sniperHW/kendynet"
	"github.com/golang/protobuf/proto"
	cs_msg "sanguo/protocol/cs/message"
	codecs "sanguo/codec/cs"
)

type Player struct {
	Name string
	Account string
	LoginTime int64
	PlayerId int64
	Session kendynet.StreamSession
}

var playerMap map[string]Player =  make(map[string]Player) //playerName player

func GetPlayerMap()map[string]Player {
	return playerMap
}

func RemovePlayerMap (name string) {
	_,ok := playerMap[name]
	if (ok) {
		delete(playerMap,name)
	}else {
		panic("remove player error.....")
	}
}

func AddPlayerMap (name string,player Player) {
	_,ok := playerMap[name]
	if (ok) {
		panic("add player error.....")
	}else {
		playerMap[name] = player
	}
}

func FindPLayerMap(name string) *Player {
	player,ok := playerMap[name]
	if (ok) {
		return &player
	}else {
		return nil
	}
}

func ForeachPlayerAiControl(stateName string) {
	for _,v := range playerMap {
		AicontrolToC := &cs_msg.AicontrolToC{}
		AicontrolToC.StateName = proto.String(stateName)
		v.Session.Send(codecs.NewMessage(0,AicontrolToC))
	}
}

func ForeachPlayerActorState(playerName string,name string) {
	for _,v := range playerMap {
		ActorstateToC := &cs_msg.ActorstateToC{}
		ActorstateToC.StateName = proto.String(name)
		ActorstateToC.ClientId = proto.String(playerName)
		v.Session.Send(codecs.NewMessage(0,ActorstateToC))
	}
}


func ForeachPlayerAiControlChange(playerName string) {
	for _,v := range playerMap {
		AicontrolchangeToC := &cs_msg.AicontrolchangeToC{}
		AicontrolchangeToC.ClientId = proto.String(playerName)
		v.Session.Send(codecs.NewMessage(0,AicontrolchangeToC))
	}
}

func ForeachPlayerActorMove(playerName string,curPost *cs_msg.Vector3,speed *cs_msg.Vector3) {
	for _,v := range playerMap {
		ActormoveToC := &cs_msg.ActormoveToC{}
		ActormoveToC.Speed.Pos1 = proto.Int32(speed.GetPos1())
		ActormoveToC.Speed.Pos2 = proto.Int32(speed.GetPos2())
		ActormoveToC.Speed.Pos3 = proto.Int32(speed.GetPos3())

		ActormoveToC.CurrentPosition.Pos1 = proto.Int32(curPost.GetPos1())
		ActormoveToC.CurrentPosition.Pos2 = proto.Int32(curPost.GetPos2())
		ActormoveToC.CurrentPosition.Pos3 = proto.Int32(curPost.GetPos3())

		ActormoveToC.ClientId = proto.String(playerName)
		v.Session.Send(codecs.NewMessage(0,ActormoveToC))
	}
}

func ForeachPlayerTakeDamage(playerName string,damgePoint uint32) {
	for _,v := range playerMap {
		TakedamageToC := &cs_msg.TakedamageToC{}
		TakedamageToC.ClientId = proto.String(playerName)
		TakedamageToC.DamagePoint = proto.Uint32(damgePoint)
		v.Session.Send(codecs.NewMessage(0,TakedamageToC))
	}
}

func GetPlayerFirstNode() *string {
	for k, _:= range playerMap {
		return &k
	}
	return nil
}

func GetPlayerMapSize() int {
	return len(playerMap)
}

