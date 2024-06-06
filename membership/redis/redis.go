package redis

import (
	"encoding/json"
)

func GetRedisError(err error) error {
	if err == nil || err.Error() == "redis: nil" {
		return nil
	} else {
		return err
	}
}

type Node struct {
	LogicAddr string `json:"logicAddr"`
	NetAddr   string `json:"netAddr"`
	Export    bool   `json:"export"`
	Available bool   `json:"available"`
}

func (n *Node) Marshal() ([]byte, error) {
	return json.Marshal(n)
}

func (n *Node) Unmarshal(data []byte) error {
	return json.Unmarshal(data, n)
}
