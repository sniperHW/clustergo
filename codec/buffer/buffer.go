package buffer

import (
	"encoding/binary"
	"errors"
)

var ErrOutOfBounds error = errors.New("out of bounds")

func AppendByte(bs []byte, v byte) []byte {
	return append(bs, v)
}

func AppendString(bs []byte, s string) []byte {
	return append(bs, s...)
}

func AppendBytes(bs []byte, bytes []byte) []byte {
	return append(bs, bytes...)
}

func AppendUint16(bs []byte, u16 uint16) []byte {
	bu := []byte{0, 0}
	binary.BigEndian.PutUint16(bu, u16)
	return AppendBytes(bs, bu)
}

func AppendUint32(bs []byte, u32 uint32) []byte {
	bu := []byte{0, 0, 0, 0}
	binary.BigEndian.PutUint32(bu, u32)
	return AppendBytes(bs, bu)
}

func AppendUint64(bs []byte, u64 uint64) []byte {
	bu := []byte{0, 0, 0, 0, 0, 0, 0, 0}
	binary.BigEndian.PutUint64(bu, u64)
	return AppendBytes(bs, bu)
}

func AppendInt16(bs []byte, i16 int16) []byte {
	return AppendUint16(bs, uint16(i16))
}

func AppendInt32(bs []byte, i32 int32) []byte {
	return AppendUint32(bs, uint32(i32))
}

func AppendInt64(bs []byte, i64 int64) []byte {
	return AppendUint64(bs, uint64(i64))
}

func AppendInt(bs []byte, i32 int) []byte {
	return AppendUint32(bs, uint32(i32))
}

type BufferReader struct {
	bs     []byte
	offset int
}

func NewReader(b []byte) BufferReader {
	return BufferReader{bs: b}
}

func (r *BufferReader) Reset(b []byte) {
	if len(b) > 0 {
		r.bs = b
	}
	r.offset = 0
}

func (r *BufferReader) GetAll() []byte {
	return r.bs[r.offset:]
}

func (r *BufferReader) GetOffset() int {
	return r.offset
}

func (r *BufferReader) IsOver() bool {
	return r.offset >= len(r.bs)
}

func (r *BufferReader) GetByte() byte {
	if r.offset+1 > len(r.bs) {
		return 0
	} else {
		ret := r.bs[r.offset]
		r.offset += 1
		return ret
	}
}

func (r *BufferReader) CheckGetByte() (byte, error) {
	if r.offset+1 > len(r.bs) {
		return 0, ErrOutOfBounds
	} else {
		ret := r.bs[r.offset]
		r.offset += 1
		return ret, nil
	}
}

func (r *BufferReader) GetUint16() uint16 {
	if r.offset+2 > len(r.bs) {
		return 0
	} else {
		ret := binary.BigEndian.Uint16(r.bs[r.offset : r.offset+2])
		r.offset += 2
		return ret
	}
}

func (r *BufferReader) CheckGetUint16() (uint16, error) {
	if r.offset+2 > len(r.bs) {
		return 0, ErrOutOfBounds
	} else {
		ret := binary.BigEndian.Uint16(r.bs[r.offset : r.offset+2])
		r.offset += 2
		return ret, nil
	}
}

func (r *BufferReader) GetInt16() int16 {
	return int16(r.GetUint16())
}

func (r *BufferReader) CheckGetInt16() (int16, error) {
	u, err := r.CheckGetUint16()
	if nil != err {
		return 0, err
	} else {
		return int16(u), nil
	}
}

func (r *BufferReader) GetUint32() uint32 {
	if r.offset+4 > len(r.bs) {
		return 0
	} else {
		ret := binary.BigEndian.Uint32(r.bs[r.offset : r.offset+4])
		r.offset += 4
		return ret
	}
}

func (r *BufferReader) CheckGetUint32() (uint32, error) {
	if r.offset+4 > len(r.bs) {
		return 0, ErrOutOfBounds
	} else {
		ret := binary.BigEndian.Uint32(r.bs[r.offset : r.offset+4])
		r.offset += 4
		return ret, nil
	}
}

func (r *BufferReader) GetInt32() int32 {
	return int32(r.GetUint32())
}

func (r *BufferReader) CheckGetInt32() (int32, error) {
	u, err := r.CheckGetUint32()
	if nil != err {
		return 0, err
	} else {
		return int32(u), nil
	}
}

func (r *BufferReader) GetUint64() uint64 {
	if r.offset+8 > len(r.bs) {
		return 0
	} else {
		ret := binary.BigEndian.Uint64(r.bs[r.offset : r.offset+8])
		r.offset += 8
		return ret
	}
}

func (r *BufferReader) CheckGetUint64() (uint64, error) {
	if r.offset+8 > len(r.bs) {
		return 0, ErrOutOfBounds
	} else {
		ret := binary.BigEndian.Uint64(r.bs[r.offset : r.offset+8])
		r.offset += 8
		return ret, nil
	}
}

func (r *BufferReader) GetInt64() int64 {
	return int64(r.GetUint64())
}

func (r *BufferReader) CheckGetInt64() (int64, error) {
	u, err := r.CheckGetUint64()
	if nil != err {
		return 0, err
	} else {
		return int64(u), nil
	}
}

func (r *BufferReader) CheckGetInt() (int, error) {
	u, err := r.CheckGetUint32()
	if nil != err {
		return 0, err
	} else {
		return int(u), nil
	}
}

func (r *BufferReader) GetString(size int) string {
	return string(r.GetBytes(size))
}

func (r *BufferReader) CheckGetString(size int) (string, error) {
	b, err := r.CheckGetBytes(size)
	if nil != err {
		return "", err
	} else {
		return string(b), nil
	}
}

func (r *BufferReader) GetBytes(size int) []byte {
	if len(r.bs)-r.offset < size {
		size = len(r.bs) - r.offset
	}
	ret := r.bs[r.offset : r.offset+size]
	r.offset += size
	return ret
}

func (r *BufferReader) CheckGetBytes(size int) ([]byte, error) {
	if len(r.bs)-r.offset < size {
		return nil, ErrOutOfBounds
	}
	ret := r.bs[r.offset : r.offset+size]
	r.offset += size
	return ret, nil
}

func (r *BufferReader) CopyBytes(size int) ([]byte, error) {
	if b, err := r.CheckGetBytes(size); nil == err {
		out := make([]byte, len(b))
		copy(out, b)
		return out, nil
	} else {
		return nil, err
	}
}
