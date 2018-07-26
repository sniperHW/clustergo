package ss

import (
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/rpc"
	"github.com/golang/protobuf/proto"
	"sanguo/codec/pb"
	"fmt"
	"os"
)


const (
	sizeLen      = 2
	sizeFlag     = 1
	sizeCmd      = 2 
	sizeRPCSeqNo = 8
)

const (
	MESSAGE  = 0x8      //普通消息
	RPCREQ   = 0x10     //RPC请求	
	RPCRESP  = 0x18     //RPC响应
	RPCERR   = 0x20     //PRC响应错误信息
)

func setCompressFlag(flag *byte) {
	*flag |= 0x80  	
}

func getCompresFlag(flag byte) bool {
	return (flag & 0x80) != 0 
}

func setMsgType(flag *byte,tt byte) {
	if tt == MESSAGE || tt == RPCREQ || tt == RPCRESP || tt == RPCERR {
		*flag |= tt
	}
}

func getMsgType(flag byte) byte {
	return flag & 0x38
}

func setNeedRPCResp(flag *byte) {
	*flag |= 0x1
}

func getNeedRPCResp(flag byte) bool {
	return (flag & 0x1) != 0
}


type Encoder struct {
	ns_msg  string
	ns_req  string
	ns_resp string
}

func NewEncoder(ns_msg,ns_req,ns_resp string) *Encoder {
	return &Encoder{ns_msg:ns_msg,ns_req:ns_req,ns_resp:ns_resp}
}

func (this *Encoder) EnCode(o interface{}) (kendynet.Message,error) {
	var pbbytes []byte
	var cmd uint32
	var err error
	var payloadLen int
	var totalLen int

	flag := byte(0)
    switch o.(type) {
    	case proto.Message:
			if pbbytes,cmd,err = pb.Marshal(this.ns_msg,o); err != nil {
				return nil,err
			}

			payloadLen = sizeFlag + sizeCmd + len(pbbytes)
			totalLen = sizeLen + payloadLen
			if uint64(totalLen) > maxPacketSize {
				return nil,fmt.Errorf("packet too large totalLen:%d",totalLen)
			}  
			
			//len + flag + cmd + pbbytes
			buff := kendynet.NewByteBuffer(totalLen)
			//写payload大小
			buff.AppendUint16(uint16(payloadLen))

			//设置普通消息标记
			setMsgType(&flag,MESSAGE)
			//写flag
			buff.AppendByte(flag)
			//写cmd
			buff.AppendUint16(uint16(cmd))
			//写数据
			buff.AppendBytes(pbbytes)

			if totalLen != len(buff.Bytes()){
				kendynet.Errorf("totalLen error %d %d\n",totalLen,len(buff.Bytes()))
				os.Exit(0)
			}

			return buff,nil			  		
    		break
    	case *rpc.RPCRequest:
    		request := o.(*rpc.RPCRequest)

			if pbbytes,cmd,err = pb.Marshal(this.ns_req,request.Arg); err != nil {
				return nil,err
			}

			payloadLen = len(pbbytes) + sizeFlag + sizeCmd + sizeRPCSeqNo
			totalLen =  sizeLen + payloadLen

			//len + flag + cmd + pbbytes + sizeRPCSeqNo
			buff := kendynet.NewByteBuffer(totalLen)
			//写payload大小
			buff.AppendUint16(uint16(payloadLen))

			//设置RPC请求标记
			setMsgType(&flag,RPCREQ)

			//如果需要返回设置返回需要返回标记
			if request.NeedResp {
				setNeedRPCResp(&flag)
			}
			//写flag
			buff.AppendByte(flag)
			//写cmd
			buff.AppendUint16(uint16(cmd))
			//写RPC序列号
			buff.AppendUint64(uint64(request.Seq))
			//写数据
			buff.AppendBytes(pbbytes)

			return buff,nil				
    		break
    	case *rpc.RPCResponse:

    		response := o.(*rpc.RPCResponse)

    		if response.Err == nil {
				if pbbytes,cmd,err = pb.Marshal(this.ns_resp,response.Ret); err != nil {
					return nil,err
				}

				payloadLen = len(pbbytes) + sizeFlag + sizeCmd + sizeRPCSeqNo
				totalLen =  sizeLen + payloadLen
				//len + flag + cmd + pbbytes + sizeRPCSeqNo
				buff := kendynet.NewByteBuffer(totalLen)
				//写payload大小
				buff.AppendUint16(uint16(payloadLen))

				//设置RPC响应标记
				setMsgType(&flag,RPCRESP)
				//写flag
				buff.AppendByte(flag)
				//写cmd
				buff.AppendUint16(uint16(cmd))
				//写RPC序列号
				buff.AppendUint64(response.Seq)

				//写数据
				buff.AppendBytes(pbbytes)
				return buff,nil	
			} else {
				errStr := response.Err.Error()

				payloadLen = len(errStr) + sizeFlag + sizeCmd + sizeRPCSeqNo
				totalLen =  sizeLen + payloadLen

				buff := kendynet.NewByteBuffer(totalLen)
				//写payload大小
				buff.AppendUint16(uint16(payloadLen))

				//设置RPC响应标记
				setMsgType(&flag,RPCERR)
				//写flag
				buff.AppendByte(flag)
				//写cmd
				buff.AppendUint16(uint16(cmd))
				//写RPC序列号
				buff.AppendUint64(response.Seq)
				//写数据
				buff.AppendString(errStr)
				return buff,nil	
			}
    		break
    	default:
    		break
    }


    return nil,fmt.Errorf("invaild object type")
}