package pb

import (
	"fmt"
	"reflect"

	"google.golang.org/protobuf/proto"
)

const maxArraySize = 65536

type PbMeta struct {
	namespace string
	nameToID  map[string]uint32
	idToMeta  map[uint32]reflect.Type //存放>65535的reflect.Type
	metaArray []reflect.Type          //0-65535直接通过数组下标获取reflect.Type
}

var nameSpace = map[string]*PbMeta{}

func getArraySize(id uint32) int {
	for i := 1; i <= 64; i++ {
		s := i * 1024
		if int(id) < s {
			return s
		}
	}
	return 0
}

func (m *PbMeta) register(msg proto.Message, id uint32) error {
	tt := reflect.TypeOf(msg)
	name := tt.String()
	if _, ok := m.nameToID[name]; ok {
		return fmt.Errorf("%s already register to namespace:%s", name, m.namespace)
	}

	m.nameToID[name] = id

	if id < maxArraySize {
		if int(id) >= len(m.metaArray) {
			metaArray := make([]reflect.Type, getArraySize(id))
			copy(metaArray, m.metaArray)
			m.metaArray = metaArray
		}
		m.metaArray[id] = tt
	} else {
		m.idToMeta[id] = tt
	}

	return nil
}

func (m *PbMeta) newMessage(id uint32) (msg proto.Message, err error) {
	if id < uint32(len(m.metaArray)) {
		tt := m.metaArray[id]
		if tt == nil {
			err = fmt.Errorf("invaild id:%d", id)
		} else {
			msg = reflect.New(tt.Elem()).Interface().(proto.Message)
		}
	} else {
		if tt, ok := m.idToMeta[id]; ok {
			msg = reflect.New(tt.Elem()).Interface().(proto.Message)
		} else {
			err = fmt.Errorf("invaild id:%d", id)
		}
	}
	return
}

// Marshal appends the wire-format encoding of o to dst, returning the new buffer, the
// message id, and any error. Appending (rather than allocating) lets callers pass a pooled
// send buffer and avoid an extra allocation on the hot path.
func (m *PbMeta) Marshal(dst []byte, o interface{}) ([]byte, uint32, error) {
	var id uint32
	var ok bool
	if id, ok = m.nameToID[reflect.TypeOf(o).String()]; !ok {
		return dst, 0, fmt.Errorf("unregister type:%s", reflect.TypeOf(o).String())
	}

	msg := o.(proto.Message)

	b, err := proto.MarshalOptions{}.MarshalAppend(dst, msg)
	if err != nil {
		return dst, 0, err
	}
	return b, id, nil
}

func (m *PbMeta) Unmarshal(id uint32, buff []byte) (msg proto.Message, err error) {
	if msg, err = m.newMessage(id); err != nil {
		return
	}

	if len(buff) > 0 {
		if err = proto.Unmarshal(buff, msg); err != nil {
			return
		}
	}

	return
}

func GetCmd(namespace string, o proto.Message) uint32 {
	if ns, ok := nameSpace[namespace]; ok {
		return ns.nameToID[reflect.TypeOf(o).String()]
	} else {
		return 0
	}
}

// 根据名字注册实例(注意函数非线程安全，需要在初始化阶段完成所有消息的Register)
func Register(namespace string, msg proto.Message, id uint32) error {

	var ns *PbMeta
	var ok bool

	if ns, ok = nameSpace[namespace]; !ok {
		ns = &PbMeta{namespace: namespace, nameToID: map[string]uint32{}, idToMeta: map[uint32]reflect.Type{}}
		nameSpace[namespace] = ns
	}

	return ns.register(msg, id)
}

func GetMeta(namespace string) *PbMeta {
	return nameSpace[namespace]
}
