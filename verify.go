package sanguo

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"time"

	"github.com/sniperHW/sanguo/addr"
	"github.com/sniperHW/sanguo/pkg/crypto"
)

var cecret_key []byte = []byte("sanguo_2022")

type loginReq struct {
	LogicAddr uint32 `json:"LogicAddr,omitempty"`
	NetAddr   string `json:"NetAddr,omitempty"`
}

func (n *node) login(sanguo *Sanguo, conn net.Conn) error {

	j, err := json.Marshal(&loginReq{
		LogicAddr: uint32(sanguo.localAddr.LogicAddr()),
		NetAddr:   sanguo.localAddr.NetAddr().String(),
	})

	if nil != err {
		return err
	}

	if j, err = crypto.AESCBCEncrypt(cecret_key, j); nil != err {
		return err
	}

	b := make([]byte, 4+len(j))
	binary.BigEndian.PutUint32(b, uint32(len(j)))
	copy(b[4:], j)

	conn.SetWriteDeadline(time.Now().Add(time.Second))
	_, err = conn.Write(b)
	conn.SetWriteDeadline(time.Time{})

	if nil != err {
		return err
	} else {
		buffer := make([]byte, 4)
		conn.SetReadDeadline(time.Now().Add(time.Second))
		_, err = io.ReadFull(conn, buffer)
		conn.SetReadDeadline(time.Time{})
		if nil != err {
			return err
		}
	}
	return nil
}

func (s *Sanguo) auth(conn net.Conn) (err error) {
	buff := make([]byte, 4)
	conn.SetReadDeadline(time.Now().Add(time.Second))
	defer conn.SetReadDeadline(time.Time{})

	_, err = io.ReadFull(conn, buff)
	if nil != err {
		return err
	}

	datasize := int(binary.BigEndian.Uint32(buff))

	buff = make([]byte, datasize)

	_, err = io.ReadFull(conn, buff)
	if nil != err {
		return err
	}

	if buff, err = crypto.AESCBCDecrypter(cecret_key, buff); nil != err {
		return err
	}

	var req loginReq

	if err = json.Unmarshal(buff, &req); nil != err {
		return err
	}

	node := s.nodeCache.getNodeByLogicAddr(addr.LogicAddr(req.LogicAddr))
	if node == nil {
		return ErrInvaildNode
	} else if node.addr.NetAddr().String() != req.NetAddr {
		return ErrNetAddrMismatch
	}

	node.Lock()
	defer node.Unlock()
	if node.dialing {
		//当前节点同时正在向对端dialing,逻辑地址小的一方放弃接受连接
		if s.localAddr.LogicAddr() < node.addr.LogicAddr() {
			logger.Errorf("(self:%v) (other:%v) connectting simultaneously", s.localAddr.LogicAddr(), node.addr.LogicAddr())
			return errors.New("connectting simultaneously")
		}
	} else if nil != node.socket {
		return ErrDuplicateConn
	}

	resp := []byte{0, 0, 0, 0}
	binary.BigEndian.PutUint32(resp, 0)

	conn.SetWriteDeadline(time.Now().Add(time.Millisecond * 5))
	defer conn.SetWriteDeadline(time.Time{})

	if _, err = conn.Write(resp); err != nil {
		return err
	} else {
		node.onEstablish(s, conn)
		return nil
	}
}
