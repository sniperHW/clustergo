package rpc

import (
	"encoding/binary"
	"errors"
)

type BytesWriter struct {
	B []byte
}

func (bw *BytesWriter) WriteBool(v bool) {
	if v {
		bw.WriteByte(byte(1))
	} else {
		bw.WriteByte(byte(0))
	}
}

func (bw *BytesWriter) WriteByte(v byte) {
	bw.B = append(bw.B, v)
}

func (bw *BytesWriter) WriteUint16(v uint16) {
	b := []byte{0, 0}
	binary.BigEndian.PutUint16(b, v)
	bw.B = append(bw.B, b...)
}

func (bw *BytesWriter) WriteUint32(v uint32) {
	b := []byte{0, 0, 0, 0}
	binary.BigEndian.PutUint32(b, v)
	bw.B = append(bw.B, b...)
}

func (bw *BytesWriter) WriteUint64(v uint64) {
	b := []byte{0, 0, 0, 0, 0, 0, 0, 0}
	binary.BigEndian.PutUint64(b, v)
	bw.B = append(bw.B, b...)
}

func (bw *BytesWriter) WriteBytes(v []byte) {
	size := len(v)
	if size > 65536 {
		size = 65536
	}
	bw.WriteUint16(uint16(size))
	if len(v) > 0 {
		bw.B = append(bw.B, v[:size]...)
	}
}

func (bw *BytesWriter) WriteString(v string) {
	bw.WriteBytes([]byte(v))
}

type BytesReader struct {
	B []byte
}

func (br *BytesReader) ReadByte() (v byte, err error) {
	if len(br.B) < 1 {
		err = errors.New("not enough data for read")
	} else {
		v = br.B[0]
		br.B = br.B[1:len(br.B)]
	}
	return v, err
}

func (br *BytesReader) ReadBool() (v bool, err error) {
	var b byte
	if b, err = br.ReadByte(); err == nil {
		v = b == byte(1)
	}
	return v, err
}

func (br *BytesReader) ReadUint16() (v uint16, err error) {
	if len(br.B) < 2 {
		err = errors.New("not enough data for read")
	} else {
		v = binary.BigEndian.Uint16(br.B)
		br.B = br.B[2:len(br.B)]
	}
	return v, err
}

func (br *BytesReader) ReadUint32() (v uint32, err error) {
	if len(br.B) < 4 {
		err = errors.New("not enough data for read")
	} else {
		v = binary.BigEndian.Uint32(br.B)
		br.B = br.B[4:len(br.B)]
	}
	return v, err
}

func (br *BytesReader) ReadUint64() (v uint64, err error) {
	if len(br.B) < 8 {
		err = errors.New("not enough data for read")
	} else {
		v = binary.BigEndian.Uint64(br.B)
		br.B = br.B[8:len(br.B)]
	}
	return v, err
}

func (br *BytesReader) ReadBytes() (v []byte, err error) {
	var size uint16
	size, err = br.ReadUint16()
	if err != nil {
		return v, err
	}
	if size > 0 {
		if len(br.B) < int(size) {
			err = errors.New("not enough data for read")
		} else {
			v = make([]byte, 0, int(size))
			v = append(v, br.B[:int(size)]...)
			br.B = br.B[int(size):len(br.B)]
		}
	}
	return v, err
}

func (br *BytesReader) ReadString() (v string, err error) {
	var size uint16
	size, err = br.ReadUint16()
	if err != nil {
		return v, err
	}
	if size > 0 {
		if len(br.B) < int(size) {
			err = errors.New("not enough data for read")
		} else {
			v = string(br.B[:int(size)])
			br.B = br.B[int(size):len(br.B)]
		}
	}
	return v, err
}
