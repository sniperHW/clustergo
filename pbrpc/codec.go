package pbrpc

import (
	"google.golang.org/protobuf/proto"
)

type Codec struct {
}

func (c *Codec) Encode(v interface{}) ([]byte, error) {
	return proto.Marshal(v.(proto.Message))
}

func (c *Codec) Decode(b []byte, v interface{}) error {
	return proto.Unmarshal(b, v.(proto.Message))
}
