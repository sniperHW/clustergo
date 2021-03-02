package ss

import (
	"fmt"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/sanguo/codec/pb"
	_ "github.com/sniperHW/sanguo/protocol/ss"
	"os"

	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/rpc"
	"github.com/sniperHW/sanguo/cluster/rpcerr"
	//"github.com/sniperHW/kendynet/util"
)

const (
	sizeLen      = 4
	sizeFlag     = 1
	sizeTo       = 4
	sizeFrom     = 4
	sizeCmd      = 2
	sizeRPCSeqNo = 8
	headerSize   = sizeLen + sizeFlag + sizeTo + sizeFrom + sizeCmd + sizeRPCSeqNo
)

const (
	RELAY        = 0x4  //跨集群透传消息
	MESSAGE      = 0x8  //普通消息
	RPCREQ       = 0x10 //RPC请求
	RPCRESP      = 0x18 //RPC响应
	RPCERR       = 0x20 //PRC响应错误信息
	COMPRESS     = 0x80
	RPC_NEEDRESP = 0x1
	MESSAGE_TYPE = 0x38
)

func setCompressFlag(flag *byte) {
	*flag |= COMPRESS
}

func getCompresFlag(flag byte) bool {
	return (flag & COMPRESS) != 0
}

func setMsgType(flag *byte, tt byte) {
	if tt == MESSAGE || tt == RPCREQ || tt == RPCRESP || tt == RPCERR {
		*flag |= tt
	}
}

func getMsgType(flag byte) byte {
	return flag & MESSAGE_TYPE
}

func setRelay(flag *byte) {
	*flag |= RELAY
}

func isRelay(flag byte) bool {
	return (flag & RELAY) != 0
}

func setNeedRPCResp(flag *byte) {
	*flag |= RPC_NEEDRESP
}

func getNeedRPCResp(flag byte) bool {
	return (flag & RPC_NEEDRESP) != 0
}

type Encoder struct {
	ns_msg  string
	ns_req  string
	ns_resp string
}

func NewEncoder(ns_msg, ns_req, ns_resp string) *Encoder {
	return &Encoder{ns_msg: ns_msg, ns_req: ns_req, ns_resp: ns_resp}
}

func (this *Encoder) encode(o interface{}, relayInfo []addr.LogicAddr) (kendynet.Message, error) {
	var pbbytes []byte
	var cmd uint32
	var err error
	var payloadLen int
	var totalLen int
	flag := byte(0)

	if nil != relayInfo {
		payloadLen = 8
		setRelay(&flag)
	}

	switch o.(type) {

	case proto.Message:

		if pbbytes, cmd, err = pb.Marshal(this.ns_msg, o); err != nil {
			return nil, err
		}

		payloadLen += (sizeFlag + sizeCmd + len(pbbytes))
		totalLen = (sizeLen + payloadLen)
		if uint64(totalLen) > maxPacketSize {
			return nil, fmt.Errorf("packet too large totalLen:%d", totalLen)
		}

		//len + flag + cmd + pbbytes
		buff := kendynet.NewByteBuffer(totalLen)
		//写payload大小
		buff.AppendUint32(uint32(payloadLen))

		//设置普通消息标记
		setMsgType(&flag, MESSAGE)
		//写flag
		buff.AppendByte(flag)

		if isRelay(flag) {
			buff.AppendUint32(uint32(relayInfo[0]))
			buff.AppendUint32(uint32(relayInfo[1]))
		}

		//写cmd
		buff.AppendUint16(uint16(cmd))
		//写数据
		buff.AppendBytes(pbbytes)

		if totalLen != len(buff.Bytes()) {
			kendynet.GetLogger().Errorf("totalLen error %d %d\n", totalLen, len(buff.Bytes()))
			os.Exit(0)
		}

		return buff, nil
		break
	case *rpc.RPCRequest:

		request := o.(*rpc.RPCRequest)

		if pbbytes, cmd, err = pb.Marshal(this.ns_req, request.Arg); err != nil {
			return nil, err
		}

		payloadLen += (len(pbbytes) + sizeFlag + sizeCmd + sizeRPCSeqNo)
		totalLen = (sizeLen + payloadLen)

		//len + flag + cmd + pbbytes + sizeRPCSeqNo
		buff := kendynet.NewByteBuffer(totalLen)
		//写payload大小
		buff.AppendUint32(uint32(payloadLen))

		//设置RPC请求标记
		setMsgType(&flag, RPCREQ)

		//如果需要返回设置返回需要返回标记
		if request.NeedResp {
			setNeedRPCResp(&flag)
		}
		//写flag
		buff.AppendByte(flag)

		if isRelay(flag) {
			buff.AppendUint32(uint32(relayInfo[0]))
			buff.AppendUint32(uint32(relayInfo[1]))
		}

		//写cmd
		buff.AppendUint16(uint16(cmd))
		//写RPC序列号
		buff.AppendUint64(uint64(request.Seq))
		//写数据
		buff.AppendBytes(pbbytes)

		return buff, nil
		break
	case *rpc.RPCResponse:

		response := o.(*rpc.RPCResponse)

		if response.Err == nil {
			if pbbytes, cmd, err = pb.Marshal(this.ns_resp, response.Ret); err != nil {
				return nil, err
			}

			payloadLen += (len(pbbytes) + sizeFlag + sizeCmd + sizeRPCSeqNo)
			totalLen = (sizeLen + payloadLen)
			//len + flag + cmd + pbbytes + sizeRPCSeqNo
			buff := kendynet.NewByteBuffer(totalLen)
			//写payload大小
			buff.AppendUint32(uint32(payloadLen))

			//设置RPC响应标记
			setMsgType(&flag, RPCRESP)
			//写flag
			buff.AppendByte(flag)

			if isRelay(flag) {
				buff.AppendUint32(uint32(relayInfo[0]))
				buff.AppendUint32(uint32(relayInfo[1]))
			}

			//写cmd
			buff.AppendUint16(uint16(cmd))
			//写RPC序列号
			buff.AppendUint64(response.Seq)

			//写数据
			buff.AppendBytes(pbbytes)
			return buff, nil
		} else {

			errStr := rpcerr.GetShortStrByError(response.Err)

			payloadLen += (len(errStr) + sizeFlag + sizeCmd + sizeRPCSeqNo)
			totalLen = (sizeLen + payloadLen)

			buff := kendynet.NewByteBuffer(totalLen)
			//写payload大小
			buff.AppendUint32(uint32(payloadLen))

			//设置RPC响应标记
			setMsgType(&flag, RPCERR)
			//写flag
			buff.AppendByte(flag)

			if isRelay(flag) {
				buff.AppendUint32(uint32(relayInfo[0]))
				buff.AppendUint32(uint32(relayInfo[1]))
			}

			//写cmd
			buff.AppendUint16(uint16(cmd))
			//写RPC序列号
			buff.AppendUint64(response.Seq)
			//写数据
			buff.AppendString(errStr)

			return buff, nil
		}
		break
	default:
		panic("error")
		break
	}
	return nil, nil
}

func (this *Encoder) enCodeRPCRelayError(msg *RPCRelayErrorMessage) kendynet.Message {

	var payloadLen int
	var totalLen int
	flag := byte(0)

	setRelay(&flag)

	errMsg := rpcerr.GetShortStrByError(msg.Err)

	payloadLen += (len(errMsg) + sizeFlag + sizeCmd + sizeRPCSeqNo + 8)
	totalLen = (sizeLen + payloadLen)

	buff := kendynet.NewByteBuffer(totalLen)
	//写payload大小
	buff.AppendUint32(uint32(payloadLen))

	//设置RPC响应标记
	setMsgType(&flag, RPCERR)
	//写flag
	buff.AppendByte(flag)

	buff.AppendUint32(uint32(msg.To))
	buff.AppendUint32(uint32(msg.From))

	//写cmd
	buff.AppendUint16(uint16(0))
	//写RPC序列号
	buff.AppendUint64(msg.Seqno)
	//写数据
	buff.AppendString(errMsg)

	return buff
}

func (this *Encoder) EnCode(o interface{}) (kendynet.Message, error) {
	switch o.(type) {
	case *Message:
		return this.encode(o.(*Message).GetData(), o.(*Message).relayInfo)
		break
	case proto.Message:
		return this.encode(o, nil)
		break
	case *rpc.RPCRequest:
		return this.encode(o, nil)
		break
	case *rpc.RPCResponse:
		return this.encode(o, nil)
		break
	case *RelayMessage:
		return kendynet.NewByteBuffer(o.(*RelayMessage).data, len(o.(*RelayMessage).data)), nil
		break
	case *RPCRelayErrorMessage:
		return this.enCodeRPCRelayError(o.(*RPCRelayErrorMessage)), nil
		break
	default:
		break
	}

	return nil, fmt.Errorf("invaild object type")
}
