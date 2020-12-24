package rpcerr

import (
	"errors"
)

var (
	Err_RPC_ServiceStoped = errors.New("the remote service is stoped")
	Err_RPC_InvaildMethod = errors.New("invaild rpc method")
	Err_RPC_RelayError    = errors.New("can not relay to target")
	Err_RPC_UnknowError   = errors.New("unknow error")
)

var Err2ShortStr map[error]string
var ShortStr2Err map[string]error

func GetErrorByShortStr(str string) error {
	err, ok := ShortStr2Err[str]
	if ok {
		return err
	} else {
		return Err_RPC_UnknowError
	}
}

func GetShortStrByError(err error) string {
	str, ok := Err2ShortStr[err]
	if ok {
		return str
	} else {
		return "unknow error"
	}
}

func registerError(err error, str string) {
	Err2ShortStr[err] = str
	ShortStr2Err[str] = err
}

func init() {
	Err2ShortStr = map[error]string{}
	ShortStr2Err = map[string]error{}
	registerError(Err_RPC_ServiceStoped, "s")
	registerError(Err_RPC_InvaildMethod, "m")
	registerError(Err_RPC_RelayError, "r")
}
