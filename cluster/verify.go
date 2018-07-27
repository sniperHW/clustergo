package cluster

import(
	"github.com/sniperHW/kendynet"
	"sanguo/common"
	"time"
	"fmt"
	"net"
	"io"
	"encoding/binary"
)

func login(end *endPoint,session kendynet.StreamSession) {
	go func() {
		conn := session.GetUnderConn().(*net.TCPConn)
		name := selfService.ToPeerID().ToString()
		buffer := kendynet.NewByteBuffer(len(name)+2)
		buffer.PutUint16(0,uint16(len(name)))
		buffer.PutString(2,name)
		conn.SetWriteDeadline(time.Now().Add(time.Second * common.HeartBeat_Timeout))
		_,err := conn.Write(buffer.Bytes())
		conn.SetWriteDeadline(time.Time{})		
		Infof("login send ok\n")
		if nil != err {
			PostTask(func () {
				dialError(end,session,err)
			})
		} else {
			
			buffer := make([]byte,512)
			var err  error	
			var ret  string		

			for {
				conn.SetReadDeadline(time.Now().Add(time.Second * common.HeartBeat_Timeout))
				_,err = io.ReadFull(conn,buffer[:2])
				if nil != err {
					break
				}

				s := binary.BigEndian.Uint16(buffer[:2])
				if s > 510 {
					err = fmt.Errorf("too large")
					break
				}	

				_,err = io.ReadFull(conn,buffer[2:2+s])
				if nil != err {
					break
				}

				ret = string(buffer[2:2+s])

				conn.SetReadDeadline(time.Time{})
				break
			}

			if nil != err {
				PostTask(func () {
					dialError(end,session,err)
				})
			} else {
				if ret == "ok" {
					PostTask(func () {
						dialOK(end,session)
					})
				} else {
					PostTask(func () {
						dialError(end,session,fmt.Errorf(ret))
					})
				}
			}
		}
	}()
}

func auth(session kendynet.StreamSession) (string,error) {
	buffer := make([]byte,512)
	var name string
	var err  error

	conn := session.GetUnderConn().(*net.TCPConn)

	for {
		conn.SetReadDeadline(time.Now().Add(time.Second * common.HeartBeat_Timeout))
		_,err = io.ReadFull(conn,buffer[:2])
		if nil != err {
			break
		}

		s := binary.BigEndian.Uint16(buffer[:2])
		if s > 510 {
			err = fmt.Errorf("too large")
			break
		}	

		_,err = io.ReadFull(conn,buffer[2:2+s])
		if nil != err {
			break
		}

		name = string(buffer[2:2+s])

		conn.SetReadDeadline(time.Time{})

		resp := kendynet.NewByteBuffer(64)
		resp.PutUint16(0,uint16(len("ok")))
		resp.PutString(2,"ok")
		conn.SetWriteDeadline(time.Now().Add(time.Second * common.HeartBeat_Timeout))
		_,err = conn.Write(resp.Bytes())
		conn.SetWriteDeadline(time.Time{})
		break
	}
	return name,err
}