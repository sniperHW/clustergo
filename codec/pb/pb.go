package pb

import (
	"github.com/golang/protobuf/proto"
	"fmt"
	"reflect"
)

type reflectInfo struct {
	tt   reflect.Type
	name string 
}

type pbMeta struct {
 	nameToID map[string]uint32
 	idToMeta map[uint32]reflectInfo	
}


var nameSpace = map[string]pbMeta{}

func newMessage(namespace string,id uint32) (msg proto.Message,err error){
   if ns,ok := nameSpace[namespace]; ok {
   		if mt,ok := ns.idToMeta[id]; ok {
          	msg = reflect.New(mt.tt.Elem()).Interface().(proto.Message)   			
   		} else {
   			err = fmt.Errorf("invaild id:%d",id)   			
   		}
   } else {
   		err = fmt.Errorf("invaild namespace:%s",namespace)
   }
   return
}

func GetNameByID(namespace string,id uint32) string {
    var ns   pbMeta
    var ok   bool
    if ns,ok = nameSpace[namespace]; !ok {
    	return ""
    }

	if mt,ok := ns.idToMeta[id]; ok {
  		return mt.name  			
	} else {
		return "" 			
	}
}

//根据名字注册实例(注意函数非线程安全，需要在初始化阶段完成所有消息的Register)
func Register(namespace string,msg proto.Message,id uint32) error {

    var ns  pbMeta
    var ok  bool

    if ns,ok = nameSpace[namespace]; !ok {
    	ns = pbMeta{nameToID:map[string]uint32{} , idToMeta:map[uint32]reflectInfo{}}
    	nameSpace[namespace] = ns
    }



	tt   := reflect.TypeOf(msg)
	name := tt.String()    

	if _,ok = ns.nameToID[name]; ok {
		return fmt.Errorf("%s already register to namespace:%s",name,namespace)
	}

	ns.nameToID[name] = id
    ns.idToMeta[id]   = reflectInfo{tt:tt, name:name}
    return nil
}


func Marshal(namespace string,o interface{}) ([]byte,uint32,error) {
   
   	var ns pbMeta
   	var id uint32
   	var ok bool

    if ns,ok = nameSpace[namespace]; !ok {
    	return nil,0,fmt.Errorf("invaild namespace:%s",namespace)
    }

	if id,ok = ns.nameToID[reflect.TypeOf(o).String()]; !ok {
		return nil,0,fmt.Errorf("unregister type:%s",reflect.TypeOf(o).String())
	}

	msg := o.(proto.Message)

	data, err := proto.Marshal(msg)
	if err != nil {
		return nil,0,err
	}
	return data,id,nil	
}

func Unmarshal(namespace string,id uint32,buff []byte) (proto.Message,error) {
	var msg proto.Message
	var err error

	if msg,err = newMessage(namespace,id); err != nil {
		return nil,err
	}

	if err = proto.Unmarshal(buff, msg); err != nil {
		return nil,err
	}

	return msg,nil
}