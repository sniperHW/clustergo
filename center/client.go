package center

import (
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/event"
	"github.com/sniperHW/kendynet/golog"
	"github.com/sniperHW/kendynet/rpc"
	connector "github.com/sniperHW/kendynet/socket/connector/tcp"
	center_proto "github.com/sniperHW/sanguo/center/protocol"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/sanguo/codec/ss"
	"github.com/sniperHW/sanguo/common"
	"reflect"
	"time"
)

var logger golog.LoggerI

var clientProcessQueue *event.EventQueue

var exportService uint32 //本节点是否暴露到服务器组外面

type centerHandler func(kendynet.StreamSession, proto.Message)

var centerHandlers = map[uint16]centerHandler{}

func RegisterCenterMsgHandler(cmd uint16, handler centerHandler) {
	if nil == handler {
		//记录日志
		logger.Errorf("Register %d failed: handler is nil\n", cmd)
		return
	}

	_, ok := centerHandlers[cmd]
	if ok {
		//记录日志
		logger.Errorf("Register %d failed: duplicate handler\n", cmd)
		return
	}

	centerHandlers[cmd] = handler
}

func dispatchCenterMsg(args []interface{}) {
	session := args[0].(kendynet.StreamSession)
	msg := args[1].(*ss.Message)

	data := msg.GetData()
	switch data.(type) {
	case *rpc.RPCResponse:

		onRPCResponse(data.(*rpc.RPCResponse))
	case proto.Message:
		cmd := msg.GetCmd()
		handler, ok := centerHandlers[cmd]
		if ok {
			handler(session, msg.GetData().(proto.Message))
		} else {
			//记录日志
			logger.Errorf("unknow cmd:%s\n", cmd)
		}
	default:
		logger.Errorf("invaild message type:%s \n", reflect.TypeOf(data).String())
	}
}

type center struct {
	addr     string
	selfAddr addr.Addr
}

func login(session kendynet.StreamSession, req *center_proto.Login, onResp func(interface{}, error)) {
	if err := asynCall(session, req, onResp); nil != err {
		panic(err)
	}
}

func (this *center) connect() {
	c, err := connector.New("tcp4", this.addr)
	if nil == err {
		go func() {
			for {
				session, err := c.Dial(time.Second * 3)
				if err != nil {
					time.Sleep(time.Millisecond * 1000)
				} else {
					session.SetReceiver(center_proto.NewReceiver())
					session.SetEncoder(center_proto.NewEncoder())

					done := make(chan struct{}, 1)

					session.SetCloseCallBack(func(sess kendynet.StreamSession, reason string) {
						logger.Infof("center disconnected %s self:%s\n", reason, this.selfAddr.Logic.String())
						done <- struct{}{}
						this.connect()
					})

					session.Start(func(event *kendynet.Event) {
						if event.EventType == kendynet.EventTypeError {
							event.Session.Close(event.Data.(error).Error(), 0)
						} else {
							msg := event.Data.(*ss.Message)
							clientProcessQueue.PostNoWait(dispatchCenterMsg, session, msg)
						}
					})

					loginReq := &center_proto.Login{
						LogicAddr:     proto.Uint32(uint32(this.selfAddr.Logic)),
						NetAddr:       proto.String(this.selfAddr.Net.String()),
						ExportService: proto.Uint32(exportService),
					}

					var onResp func(interface{}, error)

					onResp = func(r interface{}, err error) {
						if nil != err {
							if err == rpc.ErrCallTimeout {
								logger.Errorln("login timeout", this.addr)
								login(session, loginReq, onResp)
							} else {
								panic(err)
							}
						} else {
							resp := r.(*center_proto.LoginRet)
							if resp.GetErrCode() == LoginOK {
								logger.Errorln("login ok", this.addr)
								ticker := time.NewTicker(time.Second * (common.HeartBeat_Timeout / 2))
								go func() {
									for {
										select {
										case <-done:
											ticker.Stop()
											return
										case <-ticker.C:
											//发送心跳
											session.Send(&center_proto.HeartbeatToCenter{
												Timestamp: proto.Int64(time.Now().UnixNano()),
											})
										}
									}
								}()
							} else {
								panic(resp.GetMsg())
							}
						}
					}
					login(session, loginReq, onResp)
					break
				}
			}
		}()
	} else {
		logger.Errorf("NewConnector failed:%s\n", err.Error())
	}
}

func ClientInit(queue *event.EventQueue, l golog.LoggerI, export uint32) {
	clientProcessQueue = queue
	logger = l
	exportService = export
}

func ConnectCenter(centerAddrs []string, selfAddr addr.Addr) {
	centers := map[string]bool{}
	for _, v := range centerAddrs {
		if _, ok := centers[v]; !ok {
			centers[v] = true
			c := &center{
				addr:     v,
				selfAddr: selfAddr,
			}
			c.connect()
		}
	}
}
