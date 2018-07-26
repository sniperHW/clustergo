package main

import (
	"sanguo/codec/pb"
	"sanguo/codec/ss"
	"sanguo/codec/test/testproto"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet/rpc"
	"fmt"
)

func testRPCREQ () {


	req := &rpc.RPCRequest{}
	req.Seq = 1
	req.NeedResp = true
	toS := &testproto.ChatToS{}
	toS.Content = proto.String("RPC REQ")
	req.Arg = toS

	encoder := ss.NewEncoder("ss","rpc_req","rpc_resp")

	buff,err := encoder.EnCode(req)

	if nil != err {
		fmt.Println(err)
		return
	}

	if nil != buff {

		receiver := ss.NewReceiver("ss","rpc_req","rpc_resp")

		r,err := receiver.TestUnpack(buff.Bytes())

		if nil != err {
			fmt.Println(err)
			return
		}
		req = r.(*rpc.RPCRequest)
		fmt.Println(req)
	}
}


func testRPCRESP () {


	resp := &rpc.RPCResponse{}
	resp.Seq = 2
	toC := &testproto.ChatToC{}
	toC.Response = proto.String("RPC RESP")
	resp.Ret = toC

	encoder := ss.NewEncoder("ss","rpc_req","rpc_resp")

	buff,err := encoder.EnCode(resp)

	if nil != err {
		fmt.Println(err)
		return
	}

	if nil != buff {

		receiver := ss.NewReceiver("ss","rpc_req","rpc_resp")

		r,err := receiver.TestUnpack(buff.Bytes())

		if nil != err {
			fmt.Println(err)
			return
		}
		resp = r.(*rpc.RPCResponse)
		fmt.Println(resp)
	}
}

func testRPCRESPErr () {


	resp := &rpc.RPCResponse{}
	resp.Seq = 3
	resp.Err = fmt.Errorf("error")

	encoder := ss.NewEncoder("ss","rpc_req","rpc_resp")

	buff,err := encoder.EnCode(resp)

	if nil != err {
		fmt.Println(err)
		return
	}

	if nil != buff {

		receiver := ss.NewReceiver("ss","rpc_req","rpc_resp")

		r,err := receiver.TestUnpack(buff.Bytes())

		if nil != err {
			fmt.Println(err)
			return
		}
		resp = r.(*rpc.RPCResponse)
		fmt.Println(resp)
	}
}


func testMessage() {
	toS := &testproto.ChatToS{}
	toS.Content = proto.String("Message")

	encoder := ss.NewEncoder("ss","rpc_req","rpc_resp")

	buff,err := encoder.EnCode(toS)

	if nil != err {
		fmt.Println(err)
		return
	}

	if nil != buff {

		receiver := ss.NewReceiver("ss","rpc_req","rpc_resp")

		r,err := receiver.TestUnpack(buff.Bytes())

		if nil != err {
			fmt.Println(err)
			return
		}

		msg := r.(ss.Message)
		fmt.Println(msg.GetName(),msg.GetData())
	}	

}


func main() {
	pb.Register("rpc_req",&testproto.ChatToS{},1);
	pb.Register("rpc_resp",&testproto.ChatToC{},1);
	pb.Register("ss",&testproto.ChatToS{},1);
	testRPCREQ()
	testRPCRESP()
	testRPCRESPErr()
	testMessage()
}