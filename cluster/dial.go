package cluster

import (
	"fmt"
	"github.com/sniperHW/kendynet"
	connector "github.com/sniperHW/kendynet/socket/connector/tcp"
	"time"
)

var ErrDial error = fmt.Errorf("dail failed")

func dialError(end *endPoint, session kendynet.StreamSession, err error) {

	end.mtx.Lock()
	defer end.mtx.Unlock()
	if nil != session {
		session.Close(err.Error(), 0)
	}

	/*
	 * 如果end.conn != nil 表示两端同时请求建立连接，本端已经作为服务端成功接受了对端的连接
	 */
	if nil == end.conn {
		//记录日志
		Errorf("%s dial error:%s\n", end.addr.Logic.String(), err.Error())
		end.pendingMsg = end.pendingMsg[0:0]
		pendingCall := end.pendingCall
		end.pendingCall = end.pendingCall[0:0]
		queue.PostNoWait(func() {
			for _, r := range pendingCall {
				r.cb(nil, ErrDial)
			}
		})
	}
	end.dialing = false
}

func dialOK(end *endPoint, session kendynet.StreamSession) {
	end.mtx.Lock()
	defer end.mtx.Unlock()
	onEstablishClient(end, session)
	end.dialing = false
}

func dialRet(end *endPoint, session kendynet.StreamSession, err error) {
	if nil != session {
		//连接成功
		login(end, session)
	} else {
		//连接失败
		dialError(end, session, err)
	}
}

func dial(end *endPoint) {
	//发起异步Dial连接

	if !end.dialing {

		end.dialing = true

		Infof("dial %s\n", end.addr.Logic.String())

		go func() {
			client, err := connector.New("tcp4", end.addr.Net.String())
			if err != nil {
				dialRet(end, nil, err)
			} else {
				session, err := client.Dial(time.Second * 3)
				dialRet(end, session, err)
			}
		}()
	}
}
