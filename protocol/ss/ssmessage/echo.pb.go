// Code generated by protoc-gen-go. DO NOT EDIT.
// source: ss/proto/ssmessage/echo.proto

package ssmessage

import (
	fmt "fmt"
	proto "github.com/golang/protobuf/proto"
	math "math"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion3 // please upgrade the proto package

type Echo struct {
	Message              *string  `protobuf:"bytes,1,req,name=message" json:"message,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *Echo) Reset()         { *m = Echo{} }
func (m *Echo) String() string { return proto.CompactTextString(m) }
func (*Echo) ProtoMessage()    {}
func (*Echo) Descriptor() ([]byte, []int) {
	return fileDescriptor_5cfbc70f2b34e499, []int{0}
}

func (m *Echo) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Echo.Unmarshal(m, b)
}
func (m *Echo) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Echo.Marshal(b, m, deterministic)
}
func (m *Echo) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Echo.Merge(m, src)
}
func (m *Echo) XXX_Size() int {
	return xxx_messageInfo_Echo.Size(m)
}
func (m *Echo) XXX_DiscardUnknown() {
	xxx_messageInfo_Echo.DiscardUnknown(m)
}

var xxx_messageInfo_Echo proto.InternalMessageInfo

func (m *Echo) GetMessage() string {
	if m != nil && m.Message != nil {
		return *m.Message
	}
	return ""
}

func init() {
	proto.RegisterType((*Echo)(nil), "ssmessage.echo")
}

func init() { proto.RegisterFile("ss/proto/ssmessage/echo.proto", fileDescriptor_5cfbc70f2b34e499) }

var fileDescriptor_5cfbc70f2b34e499 = []byte{
	// 76 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xe2, 0x92, 0x2d, 0x2e, 0xd6, 0x2f,
	0x28, 0xca, 0x2f, 0xc9, 0xd7, 0x2f, 0x2e, 0xce, 0x4d, 0x2d, 0x2e, 0x4e, 0x4c, 0x4f, 0xd5, 0x4f,
	0x4d, 0xce, 0xc8, 0xd7, 0x03, 0x0b, 0x0a, 0x71, 0xc2, 0x45, 0x95, 0x14, 0xb8, 0x58, 0x40, 0x12,
	0x42, 0x12, 0x5c, 0xec, 0x50, 0x21, 0x09, 0x46, 0x05, 0x26, 0x0d, 0xce, 0x20, 0x18, 0x17, 0x10,
	0x00, 0x00, 0xff, 0xff, 0xdc, 0x51, 0xf4, 0x8c, 0x4c, 0x00, 0x00, 0x00,
}
