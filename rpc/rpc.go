package rpc

import (
	"context"
	"errors"
	"fmt"
	"reflect"
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

func (e *Error) IsCode(code int) bool {
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
	errEnd
)

type RequestMsg struct {
	Seq      uint64
	Method   string
	Arg      interface{}
	UserData []byte
	Oneway   bool
	codec    Codec
}

type ResponseMsg struct {
	Seq      uint64
	Err      *Error
	Ret      interface{}
	UserData []byte
	codec    Codec
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

// EncodeRequest appends the wire form of req to dst, encoding Arg via codec straight
// into dst (so a caller can pass a pooled send buffer and avoid a separate allocation).
func EncodeRequest(dst []byte, req *RequestMsg, codec Codec) ([]byte, error) {
	method := []byte(req.Method)

	if len(method) > maxMethodLen {
		method = method[:maxMethodLen]
	}

	lenUserdata := len(req.UserData)
	if len(req.UserData) > maxUserData {
		lenUserdata = maxUserData
		req.UserData = req.UserData[:lenUserdata]
	}

	bw := BytesWriter{B: dst}

	//seq
	bw.WriteUint64(req.Seq)
	//oneway
	bw.WriteBool(req.Oneway)
	//method
	bw.WriteBytes(method)
	//userdata
	bw.WriteBytes(req.UserData)
	//arg
	if err := bw.WriteMessage(codec, req.Arg); err != nil {
		return bw.B, err
	}

	return bw.B, nil
}

// DecodeRequest parses buff into a RequestMsg. codec is stashed on the message so a
// later decodeArgInto (e.g. in Register) can turn the []byte arg into the typed value.
// codec may be nil when the caller only inspects header fields (Seq/Oneway/Method).
func DecodeRequest(buff []byte, codec Codec) (*RequestMsg, error) {
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
	var arg []byte
	if arg, err = br.ReadBytes(); err != nil {
		return nil, err
	}
	req.Arg = arg
	req.codec = codec
	return &req, nil
}

// EncodeResponse appends the wire form of resp to dst, encoding Ret via codec into dst.
func EncodeResponse(dst []byte, resp *ResponseMsg, codec Codec) ([]byte, error) {
	var errByte []byte
	bw := BytesWriter{B: dst}
	lenUserdata := len(resp.UserData)
	if len(resp.UserData) > maxUserData {
		lenUserdata = maxUserData
		resp.UserData = resp.UserData[:lenUserdata]
	}

	if resp.Err != nil {
		errByte = []byte(resp.Err.str)
		if len(errByte) > maxErrStrLen {
			errByte = errByte[:maxErrStrLen]
		}
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
	if err := bw.WriteMessage(codec, resp.Ret); err != nil {
		return bw.B, err
	}
	return bw.B, nil
}

// DecodeResponse parses buff into a ResponseMsg, stashing codec for a later decodeRet.
func DecodeResponse(buff []byte, codec Codec) (*ResponseMsg, error) {
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
	var ret []byte
	if ret, err = br.ReadBytes(); err != nil {
		return nil, err
	}
	resp.Ret = ret
	resp.codec = codec
	return &resp, nil
}

// encode/decode Arg/Ret. Encode appends the marshalled form to dst (so callers can
// hand it a pooled buffer); Decode unmarshals b into v.
type Codec interface {
	Encode(dst []byte, v interface{}) ([]byte, error)
	Decode(b []byte, v interface{}) error
}

// decodeArgInto fills dst (a *Arg) with the request argument. Arg takes two forms:
//   - []byte: the wire form — decode via the codec injected by SSCodec.Decode.
//   - object: the in-process form (selfChannel / loopChannel test, no wire) — value-copy via reflection.
//   - nil:    no argument; dst is left untouched.
func (r *RequestMsg) decodeArgInto(dst interface{}) error {
	return decodeInto(r.codec, r.Arg, dst)
}

// decodeRet fills dst (a *Ret) with the response return value, same forms as decodeArgInto.
func (r *ResponseMsg) decodeRet(dst interface{}) error {
	return decodeInto(r.codec, r.Ret, dst)
}

func decodeInto(codec Codec, payload, dst interface{}) error {
	switch v := payload.(type) {
	case nil:
		return nil
	case []byte:
		if codec == nil {
			return errors.New("rpc: no codec to decode payload")
		}
		return codec.Decode(v, dst)
	default:
		return copyValue(dst, payload)
	}
}

// copyValue value-copies src into *dst via reflection. Used on the in-process path where
// the payload is already the live object and there is no codec to (un)marshal through.
func copyValue(dst, src interface{}) error {
	rv := reflect.ValueOf(dst).Elem()
	sv := reflect.ValueOf(src)
	for sv.Kind() == reflect.Ptr || sv.Kind() == reflect.Interface {
		sv = sv.Elem()
	}
	if !sv.Type().AssignableTo(rv.Type()) {
		return fmt.Errorf("rpc: payload type mismatch %s vs %s", sv.Type(), rv.Type())
	}
	rv.Set(sv)
	return nil
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
			if err = req.decodeArgInto(arg); err != nil {
				return NewError(ErrOther, fmt.Sprintf("arg decode error:%v", err))
			}
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("method:%s channel:%s %s", req.Method, replyer.channel.Name(), fmt.Sprintf("%v: %s", r, string(debug.Stack())))
					err = NewError(ErrOther, "method panic")
				}
			}()
			req.Arg = arg

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
