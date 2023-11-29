// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.31.0
// 	protoc        v4.23.4
// source: proto/echo.proto

package echo

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type EchoReq struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Msg string `protobuf:"bytes,1,opt,name=msg,proto3" json:"msg,omitempty"`
}

func (x *EchoReq) Reset() {
	*x = EchoReq{}
	if protoimpl.UnsafeEnabled {
		mi := &file_proto_echo_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *EchoReq) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*EchoReq) ProtoMessage() {}

func (x *EchoReq) ProtoReflect() protoreflect.Message {
	mi := &file_proto_echo_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use EchoReq.ProtoReflect.Descriptor instead.
func (*EchoReq) Descriptor() ([]byte, []int) {
	return file_proto_echo_proto_rawDescGZIP(), []int{0}
}

func (x *EchoReq) GetMsg() string {
	if x != nil {
		return x.Msg
	}
	return ""
}

type EchoRsp struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Msg string `protobuf:"bytes,1,opt,name=msg,proto3" json:"msg,omitempty"`
}

func (x *EchoRsp) Reset() {
	*x = EchoRsp{}
	if protoimpl.UnsafeEnabled {
		mi := &file_proto_echo_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *EchoRsp) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*EchoRsp) ProtoMessage() {}

func (x *EchoRsp) ProtoReflect() protoreflect.Message {
	mi := &file_proto_echo_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use EchoRsp.ProtoReflect.Descriptor instead.
func (*EchoRsp) Descriptor() ([]byte, []int) {
	return file_proto_echo_proto_rawDescGZIP(), []int{1}
}

func (x *EchoRsp) GetMsg() string {
	if x != nil {
		return x.Msg
	}
	return ""
}

var File_proto_echo_proto protoreflect.FileDescriptor

var file_proto_echo_proto_rawDesc = []byte{
	0x0a, 0x10, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x65, 0x63, 0x68, 0x6f, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x22, 0x1b, 0x0a, 0x07, 0x65, 0x63, 0x68, 0x6f, 0x52, 0x65, 0x71, 0x12, 0x10, 0x0a,
	0x03, 0x6d, 0x73, 0x67, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x6d, 0x73, 0x67, 0x22,
	0x1b, 0x0a, 0x07, 0x65, 0x63, 0x68, 0x6f, 0x52, 0x73, 0x70, 0x12, 0x10, 0x0a, 0x03, 0x6d, 0x73,
	0x67, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x6d, 0x73, 0x67, 0x42, 0x0e, 0x5a, 0x0c,
	0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x2f, 0x65, 0x63, 0x68, 0x6f, 0x62, 0x06, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_proto_echo_proto_rawDescOnce sync.Once
	file_proto_echo_proto_rawDescData = file_proto_echo_proto_rawDesc
)

func file_proto_echo_proto_rawDescGZIP() []byte {
	file_proto_echo_proto_rawDescOnce.Do(func() {
		file_proto_echo_proto_rawDescData = protoimpl.X.CompressGZIP(file_proto_echo_proto_rawDescData)
	})
	return file_proto_echo_proto_rawDescData
}

var file_proto_echo_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_proto_echo_proto_goTypes = []interface{}{
	(*EchoReq)(nil), // 0: echoReq
	(*EchoRsp)(nil), // 1: echoRsp
}
var file_proto_echo_proto_depIdxs = []int32{
	0, // [0:0] is the sub-list for method output_type
	0, // [0:0] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_proto_echo_proto_init() }
func file_proto_echo_proto_init() {
	if File_proto_echo_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_proto_echo_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*EchoReq); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_proto_echo_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*EchoRsp); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_proto_echo_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_proto_echo_proto_goTypes,
		DependencyIndexes: file_proto_echo_proto_depIdxs,
		MessageInfos:      file_proto_echo_proto_msgTypes,
	}.Build()
	File_proto_echo_proto = out.File
	file_proto_echo_proto_rawDesc = nil
	file_proto_echo_proto_goTypes = nil
	file_proto_echo_proto_depIdxs = nil
}
