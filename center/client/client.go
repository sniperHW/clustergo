package center

import (
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/event"
	"github.com/sniperHW/kendynet/golog"
	"github.com/sniperHW/kendynet/rpc"
	connector "github.com/sniperHW/kendynet/socket/connector/tcp"
	"github.com/sniperHW/sanguo/center/constant"
	center_proto "github.com/sniperHW/sanguo/center/protocol"
	center_rpc "github.com/sniperHW/sanguo/center/rpc"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/sanguo/codec/ss"
	"github.com/sniperHW/sanguo/common"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

type centerHandler func(kendynet.StreamSession, proto.Message)

type CenterClient struct {
	logger golog.LoggerI

	clientProcessQueue *event.EventQueue

	exportService uint32 //本节点是否暴露到服务器组外面

	centerHandlers map[uint16]centerHandler

	rpcClient *rpc.RPCClient

	closed int32

	centers []*center
}

func (this *CenterClient) RegisterCenterMsgHandler(cmd uint16, handler centerHandler) {
	if nil == handler {
		//记录日志
		this.logger.Errorf("Register %d failed: handler is nil\n", cmd)
		return
	}

	_, ok := this.centerHandlers[cmd]
	if ok {
		//记录日志
		this.logger.Errorf("Register %d failed: duplicate handler\n", cmd)
		return
	}

	this.centerHandlers[cmd] = handler
}

func (this *CenterClient) Close(sendRemoveNode bool) {
	if atomic.CompareAndSwapInt32(&this.closed, 0, 1) {
		for _, v := range this.centers {
			v.close(sendRemoveNode)
		}
	}
}

func (this *CenterClient) dispatchCenterMsg(session kendynet.StreamSession, msg *ss.Message) {

	if atomic.LoadInt32(&this.closed) == 0 {
		data := msg.GetData()
		switch data.(type) {
		case *rpc.RPCResponse:
			center_rpc.OnRPCResponse(this.rpcClient, data.(*rpc.RPCResponse))
		case proto.Message:
			cmd := msg.GetCmd()
			handler, ok := this.centerHandlers[cmd]
			if ok {
				handler(session, msg.GetData().(proto.Message))
			} else {
				//记录日志
				this.logger.Errorf("unknow cmd:%s\n", cmd)
			}
		default:
			this.logger.Errorf("invaild message type:%s \n", reflect.TypeOf(data).String())
		}
	}
}

func (this *CenterClient) ConnectCenter(centerAddrs []string, selfAddr addr.Addr) {
	centers := map[string]bool{}
	for _, v := range centerAddrs {
		if _, ok := centers[v]; !ok {
			centers[v] = true
			c := &center{
				addr:         v,
				selfAddr:     selfAddr,
				centerClient: this,
			}
			this.centers = append(this.centers, c)
			c.connect()
		}
	}
}

func New(queue *event.EventQueue, l golog.LoggerI, export uint32) *CenterClient {
	return &CenterClient{
		clientProcessQueue: queue,
		logger:             l,
		exportService:      export,
		rpcClient:          center_rpc.NewClient(),
		centerHandlers:     map[uint16]centerHandler{},
		centers:            []*center{},
	}
}

type center struct {
	sync.Mutex
	addr         string
	selfAddr     addr.Addr
	centerClient *CenterClient
	session      kendynet.StreamSession
	closed       int32
}

func login(centerClient *CenterClient, session kendynet.StreamSession, req *center_proto.Login, onResp func(interface{}, error)) error {
	return center_rpc.AsynCall(centerClient.rpcClient, session, req, onResp)
}

func (this *center) close(sendRemoveNode bool) {
	if atomic.CompareAndSwapInt32(&this.closed, 0, 1) {
		this.Lock()
		s := this.session
		this.Unlock()
		if nil != s {
			if sendRemoveNode {
				s.Send(&center_proto.RemoveNode{
					Nodes: []uint32{uint32(this.selfAddr.Logic)},
				})
			}
			s.Close("centerClient close", time.Second)
		}
	}
}

func (this *center) connect() {
	c, err := connector.New("tcp", this.addr)
	if nil == err {
		go func() {
			for atomic.LoadInt32(&this.closed) == 0 {
				session, err := c.Dial(time.Second * 3)
				if err != nil {
					time.Sleep(time.Millisecond * 1000)
				} else {
					this.Lock()
					this.session = session
					this.Unlock()
					session.SetRecvTimeout(common.HeartBeat_Timeout * time.Second)
					session.SetReceiver(center_proto.NewReceiver())
					session.SetEncoder(center_proto.NewEncoder())

					done := make(chan struct{}, 1)

					session.SetCloseCallBack(func(sess kendynet.StreamSession, reason string) {
						this.Lock()
						this.session = nil
						this.Unlock()
						this.centerClient.logger.Infof("center disconnected %s self:%s\n", reason, this.selfAddr.Logic.String())
						done <- struct{}{}
						if atomic.LoadInt32(&this.closed) == 0 {
							this.connect()
						}
					})

					session.Start(func(event *kendynet.Event) {
						if event.EventType == kendynet.EventTypeError {
							event.Session.Close(event.Data.(error).Error(), 0)
						} else {
							msg := event.Data.(*ss.Message)
							this.centerClient.clientProcessQueue.PostNoWait(this.centerClient.dispatchCenterMsg, session, msg)
						}
					})

					loginReq := &center_proto.Login{
						LogicAddr:     proto.Uint32(uint32(this.selfAddr.Logic)),
						NetAddr:       proto.String(this.selfAddr.Net.String()),
						ExportService: proto.Uint32(this.centerClient.exportService),
					}

					var onResp func(interface{}, error)

					onResp = func(r interface{}, err error) {
						if nil != err {
							if err == rpc.ErrCallTimeout {
								this.centerClient.logger.Errorln("login timeout", this.addr)
								login(this.centerClient, session, loginReq, onResp)
							} else {
								//登录center中如果出现除超时以外的错误，直接退出进程
								this.centerClient.logger.Errorln(this.addr, err)
								os.Exit(0)
							}
						} else {
							resp := r.(*center_proto.LoginRet)
							if resp.GetErrCode() == constant.LoginOK {
								this.centerClient.logger.Errorln("login ok", this.addr)
								ticker := time.NewTicker(time.Second * time.Duration(constant.HeartBeatTimeout/2))
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
					if err := login(this.centerClient, session, loginReq, onResp); nil != err {
						this.centerClient.logger.Errorln(this.addr, err)
						os.Exit(0)
					}
					break
				}
			}
		}()
	} else {
		this.centerClient.logger.Errorf("NewConnector failed:%s\n", err.Error())
	}
}
