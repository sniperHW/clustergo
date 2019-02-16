package cluster

import (
	"encoding/binary"
	"fmt"
	"github.com/sniperHW/kendynet"
	"io"
	"net"
	"sanguo/cluster/addr"
	"sanguo/common"
	"time"
)

func login(end *endPoint, session kendynet.StreamSession) {
	go func() {
		conn := session.GetUnderConn().(*net.TCPConn)
		logicAddr := selfAddr.Logic
		buffer := kendynet.NewByteBuffer(4)
		buffer.PutUint32(0, uint32(logicAddr))
		conn.SetWriteDeadline(time.Now().Add(time.Second * common.HeartBeat_Timeout))
		_, err := conn.Write(buffer.Bytes())
		conn.SetWriteDeadline(time.Time{})
		Infof("login send ok\n")
		if nil != err {
			dialError(end, session, err)
		} else {
			var err error
			buffer := make([]byte, 4)
			for {
				conn.SetReadDeadline(time.Now().Add(time.Second * common.HeartBeat_Timeout))
				_, err = io.ReadFull(conn, buffer)
				if nil != err {
					break
				}

				ret := binary.BigEndian.Uint32(buffer)

				if ret != 0 {
					err = fmt.Errorf("login failed")
				}
				break
			}

			if nil != err {
				dialError(end, session, err)
			} else {
				dialOK(end, session)
			}
		}
	}()
}

func auth(session kendynet.StreamSession) (addr.LogicAddr, error) {
	buffer := make([]byte, 4)
	var err error
	conn := session.GetUnderConn().(*net.TCPConn)

	conn.SetReadDeadline(time.Now().Add(time.Second * common.HeartBeat_Timeout))
	_, err = io.ReadFull(conn, buffer)
	if nil != err {
		session.Close(err.Error(), 0)
		return addr.LogicAddr(0), err
	}

	resp := kendynet.NewByteBuffer(4)
	resp.PutUint32(0, 0)
	conn.SetWriteDeadline(time.Now().Add(time.Second * common.HeartBeat_Timeout))
	_, err = conn.Write(resp.Bytes())

	if nil != err {
		session.Close(err.Error(), 0)
		return addr.LogicAddr(0), err
	} else {
		conn.SetWriteDeadline(time.Time{})
		return addr.LogicAddr(binary.BigEndian.Uint32(buffer)), err
	}
}
