package cluster

import (
	"encoding/binary"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	center_proto "github.com/sniperHW/sanguo/center/protocol"
	"github.com/sniperHW/sanguo/common"
	"io"
	"net"
	"time"
)

//this.selfAddr.Net.String()
func login(end *endPoint, session kendynet.StreamSession) {
	go func() {
		conn := session.GetUnderConn().(*net.TCPConn)
		logicAddr := selfAddr.Logic
		buffer := kendynet.NewByteBuffer(64)
		buffer.AppendUint32(uint32(exportService))
		buffer.AppendUint32(uint32(logicAddr))
		netAddr := selfAddr.Net.String()
		netAddrSize := len(netAddr)
		buffer.AppendUint16(uint16(netAddrSize))
		buffer.AppendString(netAddr)
		pad := make([]byte, 64-4-4-2-netAddrSize)
		buffer.AppendBytes(pad)

		conn.SetWriteDeadline(time.Now().Add(time.Second * common.HeartBeat_Timeout))
		_, err := conn.Write(buffer.Bytes())
		conn.SetWriteDeadline(time.Time{})

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

func auth(session kendynet.StreamSession) (*center_proto.NodeInfo, error) {
	buffer := make([]byte, 64)
	var err error
	conn := session.GetUnderConn().(*net.TCPConn)

	conn.SetReadDeadline(time.Now().Add(time.Second * common.HeartBeat_Timeout))
	_, err = io.ReadFull(conn, buffer)
	if nil != err {
		session.Close(err.Error(), 0)
		return nil, err
	}

	resp := kendynet.NewByteBuffer(4)
	resp.PutUint32(0, 0)
	conn.SetWriteDeadline(time.Now().Add(time.Second * common.HeartBeat_Timeout))
	_, err = conn.Write(resp.Bytes())

	if nil != err {
		session.Close(err.Error(), 0)
		return nil, err
	} else {
		conn.SetWriteDeadline(time.Time{})
		reader := kendynet.NewReader(kendynet.NewByteBuffer(buffer, 64))
		export, _ := reader.GetUint32()
		logicAddr, _ := reader.GetUint32()
		strLen, _ := reader.GetUint16()
		netAddr, _ := reader.GetString(uint64(strLen))

		return &center_proto.NodeInfo{
			LogicAddr:     proto.Uint32(logicAddr),
			NetAddr:       proto.String(netAddr),
			ExportService: proto.Uint32(export),
		}, nil
	}
}
