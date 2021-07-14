// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.23.0
// 	protoc        v3.6.0
// source: none.proto

package types

import (
	reflect "reflect"
	sync "sync"

	types "github.com/33cn/chain33/types"
	proto "github.com/golang/protobuf/proto"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// This is a compile-time assertion that a sufficiently up-to-date version
// of the legacy proto package is being used.
const _ = proto.ProtoPackageIsVersion4

type NoneAction struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Types that are assignable to Value:
	//	*NoneAction_CommitDelay
	Value isNoneAction_Value `protobuf_oneof:"value"`
	Ty    int32              `protobuf:"varint,2,opt,name=Ty,proto3" json:"Ty,omitempty"`
}

func (x *NoneAction) Reset() {
	*x = NoneAction{}
	if protoimpl.UnsafeEnabled {
		mi := &file_none_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *NoneAction) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*NoneAction) ProtoMessage() {}

func (x *NoneAction) ProtoReflect() protoreflect.Message {
	mi := &file_none_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use NoneAction.ProtoReflect.Descriptor instead.
func (*NoneAction) Descriptor() ([]byte, []int) {
	return file_none_proto_rawDescGZIP(), []int{0}
}

func (m *NoneAction) GetValue() isNoneAction_Value {
	if m != nil {
		return m.Value
	}
	return nil
}

func (x *NoneAction) GetCommitDelay() *CommitDelayTx {
	if x, ok := x.GetValue().(*NoneAction_CommitDelay); ok {
		return x.CommitDelay
	}
	return nil
}

func (x *NoneAction) GetTy() int32 {
	if x != nil {
		return x.Ty
	}
	return 0
}

type isNoneAction_Value interface {
	isNoneAction_Value()
}

type NoneAction_CommitDelay struct {
	CommitDelay *CommitDelayTx `protobuf:"bytes,1,opt,name=commitDelay,proto3,oneof"`
}

func (*NoneAction_CommitDelay) isNoneAction_Value() {}

// 提交延时交易类型
type CommitDelayTx struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	DelayTx                *types.Transaction `protobuf:"bytes,1,opt,name=delayTx,proto3" json:"delayTx,omitempty"`                                //延时交易
	RelativeDelayTime      int64              `protobuf:"varint,2,opt,name=relativeDelayTime,proto3" json:"relativeDelayTime,omitempty"`           //相对延时时长，相对区块高度/相对时间(秒)
	IsBlockHeightDelayTime bool               `protobuf:"varint,3,opt,name=isBlockHeightDelayTime,proto3" json:"isBlockHeightDelayTime,omitempty"` // 延时时间类型是否为区块高度
}

func (x *CommitDelayTx) Reset() {
	*x = CommitDelayTx{}
	if protoimpl.UnsafeEnabled {
		mi := &file_none_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *CommitDelayTx) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CommitDelayTx) ProtoMessage() {}

func (x *CommitDelayTx) ProtoReflect() protoreflect.Message {
	mi := &file_none_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CommitDelayTx.ProtoReflect.Descriptor instead.
func (*CommitDelayTx) Descriptor() ([]byte, []int) {
	return file_none_proto_rawDescGZIP(), []int{1}
}

func (x *CommitDelayTx) GetDelayTx() *types.Transaction {
	if x != nil {
		return x.DelayTx
	}
	return nil
}

func (x *CommitDelayTx) GetRelativeDelayTime() int64 {
	if x != nil {
		return x.RelativeDelayTime
	}
	return 0
}

func (x *CommitDelayTx) GetIsBlockHeightDelayTime() bool {
	if x != nil {
		return x.IsBlockHeightDelayTime
	}
	return false
}

// 提交延时交易回执
type CommitDelayLog struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Submitter    string `protobuf:"bytes,1,opt,name=submitter,proto3" json:"submitter,omitempty"`        // 提交者
	DelayTxHash  string `protobuf:"bytes,2,opt,name=delayTxHash,proto3" json:"delayTxHash,omitempty"`    // 延时交易哈希
	EndDelayTime int64  `protobuf:"varint,3,opt,name=endDelayTime,proto3" json:"endDelayTime,omitempty"` // 延时终止时刻，区块高度或区块时间
}

func (x *CommitDelayLog) Reset() {
	*x = CommitDelayLog{}
	if protoimpl.UnsafeEnabled {
		mi := &file_none_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *CommitDelayLog) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CommitDelayLog) ProtoMessage() {}

func (x *CommitDelayLog) ProtoReflect() protoreflect.Message {
	mi := &file_none_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CommitDelayLog.ProtoReflect.Descriptor instead.
func (*CommitDelayLog) Descriptor() ([]byte, []int) {
	return file_none_proto_rawDescGZIP(), []int{2}
}

func (x *CommitDelayLog) GetSubmitter() string {
	if x != nil {
		return x.Submitter
	}
	return ""
}

func (x *CommitDelayLog) GetDelayTxHash() string {
	if x != nil {
		return x.DelayTxHash
	}
	return ""
}

func (x *CommitDelayLog) GetEndDelayTime() int64 {
	if x != nil {
		return x.EndDelayTime
	}
	return 0
}

var File_none_proto protoreflect.FileDescriptor

var file_none_proto_rawDesc = []byte{
	0x0a, 0x0a, 0x6e, 0x6f, 0x6e, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x05, 0x74, 0x79,
	0x70, 0x65, 0x73, 0x1a, 0x11, 0x74, 0x72, 0x61, 0x6e, 0x73, 0x61, 0x63, 0x74, 0x69, 0x6f, 0x6e,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x5f, 0x0a, 0x0a, 0x4e, 0x6f, 0x6e, 0x65, 0x41, 0x63,
	0x74, 0x69, 0x6f, 0x6e, 0x12, 0x38, 0x0a, 0x0b, 0x63, 0x6f, 0x6d, 0x6d, 0x69, 0x74, 0x44, 0x65,
	0x6c, 0x61, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x14, 0x2e, 0x74, 0x79, 0x70, 0x65,
	0x73, 0x2e, 0x43, 0x6f, 0x6d, 0x6d, 0x69, 0x74, 0x44, 0x65, 0x6c, 0x61, 0x79, 0x54, 0x78, 0x48,
	0x00, 0x52, 0x0b, 0x63, 0x6f, 0x6d, 0x6d, 0x69, 0x74, 0x44, 0x65, 0x6c, 0x61, 0x79, 0x12, 0x0e,
	0x0a, 0x02, 0x54, 0x79, 0x18, 0x02, 0x20, 0x01, 0x28, 0x05, 0x52, 0x02, 0x54, 0x79, 0x42, 0x07,
	0x0a, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x22, 0xa3, 0x01, 0x0a, 0x0d, 0x43, 0x6f, 0x6d, 0x6d,
	0x69, 0x74, 0x44, 0x65, 0x6c, 0x61, 0x79, 0x54, 0x78, 0x12, 0x2c, 0x0a, 0x07, 0x64, 0x65, 0x6c,
	0x61, 0x79, 0x54, 0x78, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x12, 0x2e, 0x74, 0x79, 0x70,
	0x65, 0x73, 0x2e, 0x54, 0x72, 0x61, 0x6e, 0x73, 0x61, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x07,
	0x64, 0x65, 0x6c, 0x61, 0x79, 0x54, 0x78, 0x12, 0x2c, 0x0a, 0x11, 0x72, 0x65, 0x6c, 0x61, 0x74,
	0x69, 0x76, 0x65, 0x44, 0x65, 0x6c, 0x61, 0x79, 0x54, 0x69, 0x6d, 0x65, 0x18, 0x02, 0x20, 0x01,
	0x28, 0x03, 0x52, 0x11, 0x72, 0x65, 0x6c, 0x61, 0x74, 0x69, 0x76, 0x65, 0x44, 0x65, 0x6c, 0x61,
	0x79, 0x54, 0x69, 0x6d, 0x65, 0x12, 0x36, 0x0a, 0x16, 0x69, 0x73, 0x42, 0x6c, 0x6f, 0x63, 0x6b,
	0x48, 0x65, 0x69, 0x67, 0x68, 0x74, 0x44, 0x65, 0x6c, 0x61, 0x79, 0x54, 0x69, 0x6d, 0x65, 0x18,
	0x03, 0x20, 0x01, 0x28, 0x08, 0x52, 0x16, 0x69, 0x73, 0x42, 0x6c, 0x6f, 0x63, 0x6b, 0x48, 0x65,
	0x69, 0x67, 0x68, 0x74, 0x44, 0x65, 0x6c, 0x61, 0x79, 0x54, 0x69, 0x6d, 0x65, 0x22, 0x74, 0x0a,
	0x0e, 0x43, 0x6f, 0x6d, 0x6d, 0x69, 0x74, 0x44, 0x65, 0x6c, 0x61, 0x79, 0x4c, 0x6f, 0x67, 0x12,
	0x1c, 0x0a, 0x09, 0x73, 0x75, 0x62, 0x6d, 0x69, 0x74, 0x74, 0x65, 0x72, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x09, 0x73, 0x75, 0x62, 0x6d, 0x69, 0x74, 0x74, 0x65, 0x72, 0x12, 0x20, 0x0a,
	0x0b, 0x64, 0x65, 0x6c, 0x61, 0x79, 0x54, 0x78, 0x48, 0x61, 0x73, 0x68, 0x18, 0x02, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x0b, 0x64, 0x65, 0x6c, 0x61, 0x79, 0x54, 0x78, 0x48, 0x61, 0x73, 0x68, 0x12,
	0x22, 0x0a, 0x0c, 0x65, 0x6e, 0x64, 0x44, 0x65, 0x6c, 0x61, 0x79, 0x54, 0x69, 0x6d, 0x65, 0x18,
	0x03, 0x20, 0x01, 0x28, 0x03, 0x52, 0x0c, 0x65, 0x6e, 0x64, 0x44, 0x65, 0x6c, 0x61, 0x79, 0x54,
	0x69, 0x6d, 0x65, 0x42, 0x0a, 0x5a, 0x08, 0x2e, 0x2e, 0x2f, 0x74, 0x79, 0x70, 0x65, 0x73, 0x62,
	0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_none_proto_rawDescOnce sync.Once
	file_none_proto_rawDescData = file_none_proto_rawDesc
)

func file_none_proto_rawDescGZIP() []byte {
	file_none_proto_rawDescOnce.Do(func() {
		file_none_proto_rawDescData = protoimpl.X.CompressGZIP(file_none_proto_rawDescData)
	})
	return file_none_proto_rawDescData
}

var file_none_proto_msgTypes = make([]protoimpl.MessageInfo, 3)
var file_none_proto_goTypes = []interface{}{
	(*NoneAction)(nil),        // 0: types.NoneAction
	(*CommitDelayTx)(nil),     // 1: types.CommitDelayTx
	(*CommitDelayLog)(nil),    // 2: types.CommitDelayLog
	(*types.Transaction)(nil), // 3: types.Transaction
}
var file_none_proto_depIdxs = []int32{
	1, // 0: types.NoneAction.commitDelay:type_name -> types.CommitDelayTx
	3, // 1: types.CommitDelayTx.delayTx:type_name -> types.Transaction
	2, // [2:2] is the sub-list for method output_type
	2, // [2:2] is the sub-list for method input_type
	2, // [2:2] is the sub-list for extension type_name
	2, // [2:2] is the sub-list for extension extendee
	0, // [0:2] is the sub-list for field type_name
}

func init() { file_none_proto_init() }
func file_none_proto_init() {
	if File_none_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_none_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*NoneAction); i {
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
		file_none_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*CommitDelayTx); i {
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
		file_none_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*CommitDelayLog); i {
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
	file_none_proto_msgTypes[0].OneofWrappers = []interface{}{
		(*NoneAction_CommitDelay)(nil),
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_none_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   3,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_none_proto_goTypes,
		DependencyIndexes: file_none_proto_depIdxs,
		MessageInfos:      file_none_proto_msgTypes,
	}.Build()
	File_none_proto = out.File
	file_none_proto_rawDesc = nil
	file_none_proto_goTypes = nil
	file_none_proto_depIdxs = nil
}
