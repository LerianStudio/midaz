// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.4.0
// - protoc             v5.27.0
// source: proto/account/account.proto

package account

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.62.0 or later.
const _ = grpc.SupportPackageIsVersion8

const (
	AccountHandler_GetByIds_FullMethodName     = "/account.AccountHandler/GetByIds"
	AccountHandler_GetByAlias_FullMethodName   = "/account.AccountHandler/GetByAlias"
	AccountHandler_Update_FullMethodName       = "/account.AccountHandler/Update"
	AccountHandler_GetByFilters_FullMethodName = "/account.AccountHandler/GetByFilters"
)

// AccountHandlerClient is the client API for AccountHandler service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type AccountHandlerClient interface {
	GetByIds(ctx context.Context, in *ManyAccountsID, opts ...grpc.CallOption) (*ManyAccountsResponse, error)
	GetByAlias(ctx context.Context, in *ManyAccountsAlias, opts ...grpc.CallOption) (*ManyAccountsResponse, error)
	Update(ctx context.Context, in *UpdateRequest, opts ...grpc.CallOption) (*Account, error)
	GetByFilters(ctx context.Context, in *GetByFiltersRequest, opts ...grpc.CallOption) (*ManyAccountsResponse, error)
}

type accountHandlerClient struct {
	cc grpc.ClientConnInterface
}

func NewAccountHandlerClient(cc grpc.ClientConnInterface) AccountHandlerClient {
	return &accountHandlerClient{cc}
}

func (c *accountHandlerClient) GetByIds(ctx context.Context, in *ManyAccountsID, opts ...grpc.CallOption) (*ManyAccountsResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(ManyAccountsResponse)
	err := c.cc.Invoke(ctx, AccountHandler_GetByIds_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *accountHandlerClient) GetByAlias(ctx context.Context, in *ManyAccountsAlias, opts ...grpc.CallOption) (*ManyAccountsResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(ManyAccountsResponse)
	err := c.cc.Invoke(ctx, AccountHandler_GetByAlias_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *accountHandlerClient) Update(ctx context.Context, in *UpdateRequest, opts ...grpc.CallOption) (*Account, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(Account)
	err := c.cc.Invoke(ctx, AccountHandler_Update_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *accountHandlerClient) GetByFilters(ctx context.Context, in *GetByFiltersRequest, opts ...grpc.CallOption) (*ManyAccountsResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(ManyAccountsResponse)
	err := c.cc.Invoke(ctx, AccountHandler_GetByFilters_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// AccountHandlerServer is the server API for AccountHandler service.
// All implementations must embed UnimplementedAccountHandlerServer
// for forward compatibility
type AccountHandlerServer interface {
	GetByIds(context.Context, *ManyAccountsID) (*ManyAccountsResponse, error)
	GetByAlias(context.Context, *ManyAccountsAlias) (*ManyAccountsResponse, error)
	Update(context.Context, *UpdateRequest) (*Account, error)
	GetByFilters(context.Context, *GetByFiltersRequest) (*ManyAccountsResponse, error)
	mustEmbedUnimplementedAccountHandlerServer()
}

// UnimplementedAccountHandlerServer must be embedded to have forward compatible implementations.
type UnimplementedAccountHandlerServer struct {
}

func (UnimplementedAccountHandlerServer) GetByIds(context.Context, *ManyAccountsID) (*ManyAccountsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetByIds not implemented")
}
func (UnimplementedAccountHandlerServer) GetByAlias(context.Context, *ManyAccountsAlias) (*ManyAccountsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetByAlias not implemented")
}
func (UnimplementedAccountHandlerServer) Update(context.Context, *UpdateRequest) (*Account, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Update not implemented")
}
func (UnimplementedAccountHandlerServer) GetByFilters(context.Context, *GetByFiltersRequest) (*ManyAccountsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetByFilters not implemented")
}
func (UnimplementedAccountHandlerServer) mustEmbedUnimplementedAccountHandlerServer() {}

// UnsafeAccountHandlerServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to AccountHandlerServer will
// result in compilation errors.
type UnsafeAccountHandlerServer interface {
	mustEmbedUnimplementedAccountHandlerServer()
}

func RegisterAccountHandlerServer(s grpc.ServiceRegistrar, srv AccountHandlerServer) {
	s.RegisterService(&AccountHandler_ServiceDesc, srv)
}

func _AccountHandler_GetByIds_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ManyAccountsID)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AccountHandlerServer).GetByIds(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: AccountHandler_GetByIds_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AccountHandlerServer).GetByIds(ctx, req.(*ManyAccountsID))
	}
	return interceptor(ctx, in, info, handler)
}

func _AccountHandler_GetByAlias_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ManyAccountsAlias)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AccountHandlerServer).GetByAlias(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: AccountHandler_GetByAlias_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AccountHandlerServer).GetByAlias(ctx, req.(*ManyAccountsAlias))
	}
	return interceptor(ctx, in, info, handler)
}

func _AccountHandler_Update_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UpdateRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AccountHandlerServer).Update(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: AccountHandler_Update_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AccountHandlerServer).Update(ctx, req.(*UpdateRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AccountHandler_GetByFilters_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetByFiltersRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AccountHandlerServer).GetByFilters(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: AccountHandler_GetByFilters_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AccountHandlerServer).GetByFilters(ctx, req.(*GetByFiltersRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// AccountHandler_ServiceDesc is the grpc.ServiceDesc for AccountHandler service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var AccountHandler_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "account.AccountHandler",
	HandlerType: (*AccountHandlerServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetByIds",
			Handler:    _AccountHandler_GetByIds_Handler,
		},
		{
			MethodName: "GetByAlias",
			Handler:    _AccountHandler_GetByAlias_Handler,
		},
		{
			MethodName: "Update",
			Handler:    _AccountHandler_Update_Handler,
		},
		{
			MethodName: "GetByFilters",
			Handler:    _AccountHandler_GetByFilters_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "proto/account/account.proto",
}