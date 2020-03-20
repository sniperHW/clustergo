package cluster

import (
	"github.com/golang/protobuf/proto"
	center_proto "github.com/sniperHW/sanguo/center/protocol"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestDiff(t *testing.T) {

	{
		a := []*center_proto.NodeInfo{
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(1))},
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(2))},
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(3))},
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(4))},
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(5))},
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(6))},
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(7))},
		}

		b := []*center_proto.NodeInfo{
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(3))},
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(5))},
		}

		addI := []uint32{1, 2, 3, 4, 5, 6, 7}
		add, remove := diff(a, b)

		assert.Equal(t, len(add), len(addI))

		for k, v := range add {
			assert.Equal(t, v.GetLogicAddr(), addI[k])
		}

		assert.Equal(t, len(remove), 0)

	}

	{
		a := []*center_proto.NodeInfo{}
		b := []*center_proto.NodeInfo{
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(3))},
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(5))},
		}

		removeI := []uint32{3, 5}

		add, remove := diff(a, b)

		assert.Equal(t, len(add), 0)

		assert.Equal(t, len(remove), len(removeI))

		for k, v := range remove {
			assert.Equal(t, v.GetLogicAddr(), removeI[k])
		}

	}

	{
		a := []*center_proto.NodeInfo{
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(1))},
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(3))},
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(5))},
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(7))},
		}

		b := []*center_proto.NodeInfo{
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(5))},
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(3))},
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(6))},
		}

		add, remove := diff(a, b)

		addI := []uint32{1, 7}
		removeI := []uint32{6}

		assert.Equal(t, len(add), len(addI))

		for k, v := range add {
			assert.Equal(t, v.GetLogicAddr(), addI[k])
		}

		assert.Equal(t, len(remove), len(removeI))

		for k, v := range remove {
			assert.Equal(t, v.GetLogicAddr(), removeI[k])
		}

	}
}
