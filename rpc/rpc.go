package rpc

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
)

/*
 *  注意,传递给RPC模块的所有回调函数可能在底层信道的接收/发送goroutine上执行，
 *  为了避免接收/发送goroutine被阻塞，回调函数中不能调用阻塞函数。
 *  如需调用阻塞函数，请在回调中启动一个goroutine来执行
 */

type Error struct {
	code int
	str  string
}

func NewError(code int, err string) *Error {
	if code <= 0 || code >= errEnd {
		return nil
	} else {
		return &Error{code: code, str: err}
	}
}

func (e *Error) Error() string {
	return e.str
}

func (e *Error) Is(code int) bool {
	return e.code == code
}

const (
	ErrOk = iota
	ErrInvaildMethod
	ErrTimeout
	ErrCancel
	ErrMethod
	ErrOther
	ErrServiceUnavaliable
	ErrDisconnet
	errEnd
)

type RequestMsg struct {
	Seq      uint64
	Method   string
	Arg      []byte
	UserData []byte
	Oneway   bool
	arg      interface{}
}

func (r RequestMsg) GetArg() interface{} {
	return r.arg
}

type ResponseMsg struct {
	Seq      uint64
	Err      *Error
	Ret      []byte
	UserData []byte
}

const (
	lenSeq       = 8
	lenOneWay    = 1
	lenMethod    = 2
	lenUserData  = 2
	lenErrCode   = 2
	lenErrStr    = 2
	maxUserData  = 65536
	maxMethodLen = 65536
	maxErrStrLen = 65536
	reqHdrLen    = lenSeq + lenOneWay + lenMethod + lenUserData // seq + oneway + len(method) + len(userdata)
	respHdrLen   = lenSeq + lenErrCode + lenUserData            //seq + Error.Err.Code + len(userdata)
)

func EncodeRequest(req *RequestMsg) []byte {
	method := []byte(req.Method)

	if len(method) > maxMethodLen {
		method = method[:maxMethodLen]
	}

	lenUserdata := len(req.UserData)
	if len(req.UserData) > maxUserData {
		lenUserdata = maxUserData
		req.UserData = req.UserData[:lenUserdata]
	}

	bw := BytesWriter{
		B: make([]byte, 0, reqHdrLen+len(method)+lenUserdata+len(req.Arg)),
	}

	//seq
	bw.WriteUint64(req.Seq)
	//oneway
	bw.WriteBool(req.Oneway)
	//method
	bw.WriteBytes(method)
	//userdata
	bw.WriteBytes(req.UserData)
	//arg
	bw.WriteBytes(req.Arg)

	return bw.B
}

func DecodeRequest(buff []byte) (*RequestMsg, error) {
	var req RequestMsg
	var err error
	br := BytesReader{
		B: buff,
	}
	if req.Seq, err = br.ReadUint64(); err != nil {
		return nil, err
	}
	if req.Oneway, err = br.ReadBool(); err != nil {
		return nil, err
	}
	if req.Method, err = br.ReadString(); err != nil {
		return nil, err
	}
	if req.UserData, err = br.ReadBytes(); err != nil {
		return nil, err
	}
	if req.Arg, err = br.ReadBytes(); err != nil {
		return nil, err
	}
	return &req, nil
}

func EncodeResponse(resp *ResponseMsg) []byte {
	var errByte []byte
	var bw BytesWriter
	lenUserdata := len(resp.UserData)
	if len(resp.UserData) > maxUserData {
		lenUserdata = maxUserData
		resp.UserData = resp.UserData[:lenUserdata]
	}

	if resp.Err == nil {
		bw.B = make([]byte, 0, respHdrLen+len(resp.Ret)+lenUserdata)
	} else {
		errByte = []byte(resp.Err.str)
		if len(errByte) > maxErrStrLen {
			errByte = errByte[:maxErrStrLen]
		}
		bw.B = make([]byte, 0, respHdrLen+lenErrStr+len(errByte)+len(resp.Ret)+lenUserdata)
	}
	//seq
	bw.WriteUint64(resp.Seq)
	//usedata
	bw.WriteBytes(resp.UserData)
	//err
	if resp.Err != nil {
		bw.WriteUint16(uint16(resp.Err.code))
		bw.WriteBytes(errByte)
	} else {
		bw.WriteUint16(uint16(0))
	}
	//ret
	bw.WriteBytes(resp.Ret)
	return bw.B
}

func DecodeResponse(buff []byte) (*ResponseMsg, error) {
	var resp ResponseMsg
	var err error
	var errCode uint16
	var errStr string
	br := BytesReader{
		B: buff,
	}
	if resp.Seq, err = br.ReadUint64(); err != nil {
		return nil, err
	}
	if resp.UserData, err = br.ReadBytes(); err != nil {
		return nil, err
	}
	if errCode, err = br.ReadUint16(); err != nil {
		return nil, err
	}
	if errCode != 0 {
		if errStr, err = br.ReadString(); err != nil {
			return nil, err
		}
		resp.Err = NewError(int(errCode), errStr)
	}
	if resp.Ret, err = br.ReadBytes(); err != nil {
		return nil, err
	}
	return &resp, nil
}

// encode/decode Arg/Ret
type Codec interface {
	Encode(interface{}) ([]byte, error)
	Decode([]byte, interface{}) error
}

type Channel interface {
	Request(*RequestMsg) error
	RequestWithContext(context.Context, *RequestMsg) error
	Reply(*ResponseMsg) error
	Name() string
	IsRetryAbleError(error) bool
}

func Register[Arg any](s *Server, name string, method func(context.Context, *Replyer, *Arg)) error {
	s.Lock()
	defer s.Unlock()
	if name == "" {
		return errors.New("RegisterMethod nams is nil")
	} else if _, ok := s.methods[name]; ok {
		return fmt.Errorf("duplicate method:%s", name)
	} else {

		caller := func(c context.Context, s *Server, req *RequestMsg, replyer *Replyer) (err error) {
			arg := new(Arg)
			if err = s.codec.Decode(req.Arg, arg); err != nil {
				return NewError(ErrOther, fmt.Sprintf("arg decode error:%v", err))
			}
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("method:%s channel:%s %s", req.Method, replyer.channel.Name(), fmt.Sprintf("%v: %s", r, string(debug.Stack())))
					err = NewError(ErrOther, "method panic")
				}
			}()
			req.arg = arg

			for _, v := range s.inInterceptor {
				if !v(replyer, req) {
					return
				}
			}
			method(c, replyer, arg)
			return nil
		}
		s.methods[name] = caller
		return nil
	}
}

func UnRegister(s *Server, name string) {
	s.Lock()
	defer s.Unlock()
	delete(s.methods, name)
}
