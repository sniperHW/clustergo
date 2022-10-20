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

func (this *BufferReader) Reset(b []byte) {
	if len(b) > 0 {
		this.bs = b
	}
	this.offset = 0
}

func (this *BufferReader) GetAll() []byte {
	return this.bs[this.offset:]
}

func (this *BufferReader) GetOffset() int {
	return this.offset
}

func (this *BufferReader) IsOver() bool {
	return this.offset >= len(this.bs)
}

func (this *BufferReader) GetByte() byte {
	if this.offset+1 > len(this.bs) {
		return 0
	} else {
		ret := this.bs[this.offset]
		this.offset += 1
		return ret
	}
}

func (this *BufferReader) CheckGetByte() (byte, error) {
	if this.offset+1 > len(this.bs) {
		return 0, ErrOutOfBounds
	} else {
		ret := this.bs[this.offset]
		this.offset += 1
		return ret, nil
	}
}

func (this *BufferReader) GetUint16() uint16 {
	if this.offset+2 > len(this.bs) {
		return 0
	} else {
		ret := binary.BigEndian.Uint16(this.bs[this.offset : this.offset+2])
		this.offset += 2
		return ret
	}
}

func (this *BufferReader) CheckGetUint16() (uint16, error) {
	if this.offset+2 > len(this.bs) {
		return 0, ErrOutOfBounds
	} else {
		ret := binary.BigEndian.Uint16(this.bs[this.offset : this.offset+2])
		this.offset += 2
		return ret, nil
	}
}

func (this *BufferReader) GetInt16() int16 {
	return int16(this.GetUint16())
}

func (this *BufferReader) CheckGetInt16() (int16, error) {
	u, err := this.CheckGetUint16()
	if nil != err {
		return 0, err
	} else {
		return int16(u), nil
	}
}

func (this *BufferReader) GetUint32() uint32 {
	if this.offset+4 > len(this.bs) {
		return 0
	} else {
		ret := binary.BigEndian.Uint32(this.bs[this.offset : this.offset+4])
		this.offset += 4
		return ret
	}
}

func (this *BufferReader) CheckGetUint32() (uint32, error) {
	if this.offset+4 > len(this.bs) {
		return 0, ErrOutOfBounds
	} else {
		ret := binary.BigEndian.Uint32(this.bs[this.offset : this.offset+4])
		this.offset += 4
		return ret, nil
	}
}

func (this *BufferReader) GetInt32() int32 {
	return int32(this.GetUint32())
}

func (this *BufferReader) CheckGetInt32() (int32, error) {
	u, err := this.CheckGetUint32()
	if nil != err {
		return 0, err
	} else {
		return int32(u), nil
	}
}

func (this *BufferReader) GetUint64() uint64 {
	if this.offset+8 > len(this.bs) {
		return 0
	} else {
		ret := binary.BigEndian.Uint64(this.bs[this.offset : this.offset+8])
		this.offset += 8
		return ret
	}
}

func (this *BufferReader) CheckGetUint64() (uint64, error) {
	if this.offset+8 > len(this.bs) {
		return 0, ErrOutOfBounds
	} else {
		ret := binary.BigEndian.Uint64(this.bs[this.offset : this.offset+8])
		this.offset += 8
		return ret, nil
	}
}

func (this *BufferReader) GetInt64() int64 {
	return int64(this.GetUint64())
}

func (this *BufferReader) CheckGetInt64() (int64, error) {
	u, err := this.CheckGetUint64()
	if nil != err {
		return 0, err
	} else {
		return int64(u), nil
	}
}

func (this *BufferReader) CheckGetInt() (int, error) {
	u, err := this.CheckGetUint32()
	if nil != err {
		return 0, err
	} else {
		return int(u), nil
	}
}

func (this *BufferReader) GetString(size int) string {
	return string(this.GetBytes(size))
}

func (this *BufferReader) CheckGetString(size int) (string, error) {
	b, err := this.CheckGetBytes(size)
	if nil != err {
		return "", err
	} else {
		return string(b), nil
	}
}

func (this *BufferReader) GetBytes(size int) []byte {
	if len(this.bs)-this.offset < size {
		size = len(this.bs) - this.offset
	}
	ret := this.bs[this.offset : this.offset+size]
	this.offset += size
	return ret
}

func (this *BufferReader) CheckGetBytes(size int) ([]byte, error) {
	if len(this.bs)-this.offset < size {
		return nil, ErrOutOfBounds
	}
	ret := this.bs[this.offset : this.offset+size]
	this.offset += size
	return ret, nil
}

func (this *BufferReader) CopyBytes(size int) ([]byte, error) {
	if b, err := this.CheckGetBytes(size); nil == err {
		out := make([]byte, len(b))
		copy(out, b)
		return out, nil
	} else {
		return nil, err
	}
}
