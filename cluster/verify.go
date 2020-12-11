package cluster

import (
	"encoding/binary"
	"fmt"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/sanguo/common"
	"io"
	"net"
	"time"
)

func (this *Cluster) login(end *endPoint, session kendynet.StreamSession, counter int) {
	go func() {
		conn := session.GetUnderConn().(*net.TCPConn)
		logicAddr := this.serverState.selfAddr.Logic
		buffer := kendynet.NewByteBuffer(64)
		buffer.AppendUint32(uint32(logicAddr))
		netAddr := this.serverState.selfAddr.Net.String()
		netAddrSize := len(netAddr)
		buffer.AppendUint16(uint16(netAddrSize))
		buffer.AppendString(netAddr)
		pad := make([]byte, 64-4-2-netAddrSize)
		buffer.AppendBytes(pad)

		conn.SetWriteDeadline(time.Now().Add(time.Second * common.HeartBeat_Timeout))
		_, err := conn.Write(buffer.Bytes())
		conn.SetWriteDeadline(time.Time{})

		if nil != err {
			this.dialError(end, session, err, counter)
		} else {
			var err error
			buffer := make([]byte, 4)
			for {
				conn.SetReadDeadline(time.Now().Add(time.Second * common.HeartBeat_Timeout))
				_, err = io.ReadFull(conn, buffer)
				if nil != err {
					break
				}
				conn.SetReadDeadline(time.Time{})

				ret := binary.BigEndian.Uint32(buffer)

				if ret != 0 {
					err = fmt.Errorf("login failed")
				}
				break
			}

			if nil != err {
				this.dialError(end, session, err, counter)
			} else {
				this.dialOK(end, session)
			}
		}
	}()
}

func (this *Cluster) auth(session kendynet.StreamSession) (*endPoint, error) {
	buffer := make([]byte, 64)
	var err error
	conn := session.GetUnderConn().(*net.TCPConn)

	conn.SetReadDeadline(time.Now().Add(time.Second * common.HeartBeat_Timeout))
	_, err = io.ReadFull(conn, buffer)
	if nil != err {
		return nil, err
	}
	conn.SetReadDeadline(time.Time{})

	reader := kendynet.NewReader(kendynet.NewByteBuffer(buffer, 64))
	logicAddr, _ := reader.GetUint32()

	end, err := func() (*endPoint, error) {
		end := this.serviceMgr.getEndPoint(addr.LogicAddr(logicAddr))
		if nil == end {
			return nil, ERR_INVAILD_ENDPOINT
		}

		end.Lock()
		defer end.Unlock()

		if end.session != nil {
			/*
			 *如果end.session != nil 表示两端同时请求建立连接，本端已经作为客户端成功与对端建立连接
			 */
			logger.Infoln("auth dup", end.addr.Logic.String())
			return nil, ERR_DUP_CONN
		}

		return end, nil
	}()

	//send resp
	resp := kendynet.NewByteBuffer(4)
	if nil == err {
		resp.PutUint32(0, 0)
	} else {
		resp.PutUint32(0, 1)
	}

	conn.SetWriteDeadline(time.Now().Add(time.Second * common.HeartBeat_Timeout))
	_, sendErr := conn.Write(resp.Bytes())
	conn.SetWriteDeadline(time.Time{})

	if nil == sendErr {
		return end, err
	} else {
		if nil == err {
			return nil, err
		} else {
			return nil, sendErr
		}
	}

}
