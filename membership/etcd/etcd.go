package etcd

type Node struct {
	LogicAddr string
	NetAddr   string
	Export    bool
	Available bool
}
