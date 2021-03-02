package cluster

import (
	"github.com/sniperHW/kendynet"
	//connector "github.com/sniperHW/kendynet/socket/connector/tcp"
	"github.com/sniperHW/kendynet/timer"
	"github.com/sniperHW/sanguo/network"
	"time"
)

func (this *Cluster) dialError(end *endPoint, session kendynet.StreamSession, err error, counter int) {

	isOk := end == this.serviceMgr.getEndPoint(end.addr.Logic)

	end.Lock()
	defer end.Unlock()

	if nil != session {
		session.Close(err, 0)
	}

	end.dialing = false

	/*
	 * 如果end.session != nil 表示两端同时请求建立连接，本端已经作为服务端成功接受了对端的连接
	 */

	if nil == end.session {
		//记录日志
		logger.Errorf("%s dial error:%s\n", end.addr.Logic.String(), err.Error())
		if isOk && counter < dialTerminateCount {
			end.dialing = true
			this.RegisterTimerOnce(time.Second, func(t *timer.Timer, _ interface{}) {
				this._dial(end, counter+1)
			}, nil)
		} else {

			end.pendingMsg = end.pendingMsg[0:0]
			pendingCall := end.pendingCall
			end.pendingCall = end.pendingCall[0:0]

			if err != ERR_INVAILD_ENDPOINT || err != ERR_AUTH {
				err = ERR_DIAL
			}

			for _, r := range pendingCall {
				if r.dialTimer.Cancel() {
					r.cb(nil, err)
				}
			}
		}
	}
}

func (this *Cluster) dialOK(end *endPoint, session kendynet.StreamSession) {
	if end == this.serviceMgr.getEndPoint(end.addr.Logic) {
		this.onEstablishClient(end, session)
	} else {
		//不再是合法的end
		this.dialError(end, session, ERR_INVAILD_ENDPOINT, dialTerminateCount)
	}
}

func (this *Cluster) _dial(end *endPoint, counter int) {

	logger.Infof("dial %s %v\n", end.addr.Logic.String(), end.addr.Net)
	go func() {
		conn, err := network.Dial("tcp", end.addr.Net.String(), time.Second*3)
		if err != nil {
			this.dialError(end, nil, err, counter)
		} else {
			this.login(end, conn, counter)
		}
	}()
}

func (this *Cluster) dial(end *endPoint, counter int) {
	//发起异步Dial连接
	if !end.dialing {
		end.dialing = true
		this._dial(end, counter)
	}
}
