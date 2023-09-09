package pb

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPB(t *testing.T) {

	Register("test", &Echo1{}, 1)

	meta := GetMeta("test")
	{
		assert.Equal(t, 1024, len(meta.metaArray))

		msg, err := meta.newMessage(1)

		assert.Nil(t, err)

		assert.Equal(t, reflect.TypeOf(msg), reflect.TypeOf(&Echo1{}))

		msg, err = meta.newMessage(2)

		assert.Nil(t, msg)

		assert.Equal(t, err.Error(), "invaild id:2")
	}
	{
		Register("test", &Echo2{}, 1024+1)

		assert.Equal(t, 1024*2, len(meta.metaArray))

		msg, err := meta.newMessage(1024 + 1)

		assert.Nil(t, err)

		assert.Equal(t, reflect.TypeOf(msg), reflect.TypeOf(&Echo2{}))

		msg, err = meta.newMessage(1024 + 2)

		assert.Nil(t, msg)

		assert.Equal(t, err.Error(), "invaild id:1026")
	}

	{
		Register("test", &Echo3{}, 2048+1)

		assert.Equal(t, 1024*3, len(meta.metaArray))

		msg, err := meta.newMessage(2048 + 1)

		assert.Nil(t, err)

		assert.Equal(t, reflect.TypeOf(msg), reflect.TypeOf(&Echo3{}))

		msg, err = meta.newMessage(2048 + 2)

		assert.Nil(t, msg)

		assert.Equal(t, err.Error(), "invaild id:2050")
	}

	{
		Register("test", &Echo4{}, 16383)

		assert.Equal(t, 16384, len(meta.metaArray))

		msg, err := meta.newMessage(16383)

		assert.Nil(t, err)

		assert.Equal(t, reflect.TypeOf(msg), reflect.TypeOf(&Echo4{}))

		msg, err = meta.newMessage(16381)

		assert.Nil(t, msg)

		assert.Equal(t, err.Error(), "invaild id:16381")
	}

	{
		Register("test", &Echo5{}, 65530)

		assert.Equal(t, 65536, len(meta.metaArray))

		msg, err := meta.newMessage(65530)

		assert.Nil(t, err)

		assert.Equal(t, reflect.TypeOf(msg), reflect.TypeOf(&Echo5{}))

		msg, err = meta.newMessage(65531)

		assert.Nil(t, msg)

		assert.Equal(t, err.Error(), "invaild id:65531")
	}

	{
		//超过65535保存在idToMeta
		Register("test", &Echo6{}, 65536)

		assert.Equal(t, 65536, len(meta.metaArray))

		assert.Equal(t, 1, len(meta.idToMeta))

		msg, err := meta.newMessage(65536)

		assert.Nil(t, err)

		assert.Equal(t, reflect.TypeOf(msg), reflect.TypeOf(&Echo6{}))

		msg, err = meta.newMessage(65540)

		assert.Nil(t, msg)

		assert.Equal(t, err.Error(), "invaild id:65540")
	}

}
