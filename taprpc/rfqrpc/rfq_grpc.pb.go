// Code generated by protoc-gen-go-grpc. DO NOT EDIT.

package rfqrpc

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// RfqClient is the client API for Rfq service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type RfqClient interface {
	// tapcli: `rfq buyorder`
	// AddAssetBuyOrder is used to add a buy order for a specific asset. If a buy
	// order already exists for the asset, it will be updated.
	AddAssetBuyOrder(ctx context.Context, in *AddAssetBuyOrderRequest, opts ...grpc.CallOption) (*AddAssetBuyOrderResponse, error)
	// tapcli: `rfq sellorder`
	// AddAssetSellOrder is used to add a sell order for a specific asset. If a
	// sell order already exists for the asset, it will be updated.
	AddAssetSellOrder(ctx context.Context, in *AddAssetSellOrderRequest, opts ...grpc.CallOption) (*AddAssetSellOrderResponse, error)
	// tapcli: `rfq selloffer`
	// AddAssetSellOffer is used to add a sell offer for a specific asset. If a
	// sell offer already exists for the asset, it will be updated.
	AddAssetSellOffer(ctx context.Context, in *AddAssetSellOfferRequest, opts ...grpc.CallOption) (*AddAssetSellOfferResponse, error)
	// tapcli: `rfq peeracceptedquotes`
	// QueryPeerAcceptedQuotes is used to query for quotes that were requested by
	// our node and have been accepted our peers.
	QueryPeerAcceptedQuotes(ctx context.Context, in *QueryPeerAcceptedQuotesRequest, opts ...grpc.CallOption) (*QueryPeerAcceptedQuotesResponse, error)
	// SubscribeRfqEventNtfns is used to subscribe to RFQ events.
	SubscribeRfqEventNtfns(ctx context.Context, in *SubscribeRfqEventNtfnsRequest, opts ...grpc.CallOption) (Rfq_SubscribeRfqEventNtfnsClient, error)
}

type rfqClient struct {
	cc grpc.ClientConnInterface
}

func NewRfqClient(cc grpc.ClientConnInterface) RfqClient {
	return &rfqClient{cc}
}

func (c *rfqClient) AddAssetBuyOrder(ctx context.Context, in *AddAssetBuyOrderRequest, opts ...grpc.CallOption) (*AddAssetBuyOrderResponse, error) {
	out := new(AddAssetBuyOrderResponse)
	err := c.cc.Invoke(ctx, "/rfqrpc.Rfq/AddAssetBuyOrder", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *rfqClient) AddAssetSellOrder(ctx context.Context, in *AddAssetSellOrderRequest, opts ...grpc.CallOption) (*AddAssetSellOrderResponse, error) {
	out := new(AddAssetSellOrderResponse)
	err := c.cc.Invoke(ctx, "/rfqrpc.Rfq/AddAssetSellOrder", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *rfqClient) AddAssetSellOffer(ctx context.Context, in *AddAssetSellOfferRequest, opts ...grpc.CallOption) (*AddAssetSellOfferResponse, error) {
	out := new(AddAssetSellOfferResponse)
	err := c.cc.Invoke(ctx, "/rfqrpc.Rfq/AddAssetSellOffer", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *rfqClient) QueryPeerAcceptedQuotes(ctx context.Context, in *QueryPeerAcceptedQuotesRequest, opts ...grpc.CallOption) (*QueryPeerAcceptedQuotesResponse, error) {
	out := new(QueryPeerAcceptedQuotesResponse)
	err := c.cc.Invoke(ctx, "/rfqrpc.Rfq/QueryPeerAcceptedQuotes", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *rfqClient) SubscribeRfqEventNtfns(ctx context.Context, in *SubscribeRfqEventNtfnsRequest, opts ...grpc.CallOption) (Rfq_SubscribeRfqEventNtfnsClient, error) {
	stream, err := c.cc.NewStream(ctx, &Rfq_ServiceDesc.Streams[0], "/rfqrpc.Rfq/SubscribeRfqEventNtfns", opts...)
	if err != nil {
		return nil, err
	}
	x := &rfqSubscribeRfqEventNtfnsClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type Rfq_SubscribeRfqEventNtfnsClient interface {
	Recv() (*RfqEvent, error)
	grpc.ClientStream
}

type rfqSubscribeRfqEventNtfnsClient struct {
	grpc.ClientStream
}

func (x *rfqSubscribeRfqEventNtfnsClient) Recv() (*RfqEvent, error) {
	m := new(RfqEvent)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// RfqServer is the server API for Rfq service.
// All implementations must embed UnimplementedRfqServer
// for forward compatibility
type RfqServer interface {
	// tapcli: `rfq buyorder`
	// AddAssetBuyOrder is used to add a buy order for a specific asset. If a buy
	// order already exists for the asset, it will be updated.
	AddAssetBuyOrder(context.Context, *AddAssetBuyOrderRequest) (*AddAssetBuyOrderResponse, error)
	// tapcli: `rfq sellorder`
	// AddAssetSellOrder is used to add a sell order for a specific asset. If a
	// sell order already exists for the asset, it will be updated.
	AddAssetSellOrder(context.Context, *AddAssetSellOrderRequest) (*AddAssetSellOrderResponse, error)
	// tapcli: `rfq selloffer`
	// AddAssetSellOffer is used to add a sell offer for a specific asset. If a
	// sell offer already exists for the asset, it will be updated.
	AddAssetSellOffer(context.Context, *AddAssetSellOfferRequest) (*AddAssetSellOfferResponse, error)
	// tapcli: `rfq peeracceptedquotes`
	// QueryPeerAcceptedQuotes is used to query for quotes that were requested by
	// our node and have been accepted our peers.
	QueryPeerAcceptedQuotes(context.Context, *QueryPeerAcceptedQuotesRequest) (*QueryPeerAcceptedQuotesResponse, error)
	// SubscribeRfqEventNtfns is used to subscribe to RFQ events.
	SubscribeRfqEventNtfns(*SubscribeRfqEventNtfnsRequest, Rfq_SubscribeRfqEventNtfnsServer) error
	mustEmbedUnimplementedRfqServer()
}

// UnimplementedRfqServer must be embedded to have forward compatible implementations.
type UnimplementedRfqServer struct {
}

func (UnimplementedRfqServer) AddAssetBuyOrder(context.Context, *AddAssetBuyOrderRequest) (*AddAssetBuyOrderResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method AddAssetBuyOrder not implemented")
}
func (UnimplementedRfqServer) AddAssetSellOrder(context.Context, *AddAssetSellOrderRequest) (*AddAssetSellOrderResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method AddAssetSellOrder not implemented")
}
func (UnimplementedRfqServer) AddAssetSellOffer(context.Context, *AddAssetSellOfferRequest) (*AddAssetSellOfferResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method AddAssetSellOffer not implemented")
}
func (UnimplementedRfqServer) QueryPeerAcceptedQuotes(context.Context, *QueryPeerAcceptedQuotesRequest) (*QueryPeerAcceptedQuotesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method QueryPeerAcceptedQuotes not implemented")
}
func (UnimplementedRfqServer) SubscribeRfqEventNtfns(*SubscribeRfqEventNtfnsRequest, Rfq_SubscribeRfqEventNtfnsServer) error {
	return status.Errorf(codes.Unimplemented, "method SubscribeRfqEventNtfns not implemented")
}
func (UnimplementedRfqServer) mustEmbedUnimplementedRfqServer() {}

// UnsafeRfqServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to RfqServer will
// result in compilation errors.
type UnsafeRfqServer interface {
	mustEmbedUnimplementedRfqServer()
}

func RegisterRfqServer(s grpc.ServiceRegistrar, srv RfqServer) {
	s.RegisterService(&Rfq_ServiceDesc, srv)
}

func _Rfq_AddAssetBuyOrder_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(AddAssetBuyOrderRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RfqServer).AddAssetBuyOrder(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/rfqrpc.Rfq/AddAssetBuyOrder",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RfqServer).AddAssetBuyOrder(ctx, req.(*AddAssetBuyOrderRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Rfq_AddAssetSellOrder_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(AddAssetSellOrderRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RfqServer).AddAssetSellOrder(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/rfqrpc.Rfq/AddAssetSellOrder",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RfqServer).AddAssetSellOrder(ctx, req.(*AddAssetSellOrderRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Rfq_AddAssetSellOffer_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(AddAssetSellOfferRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RfqServer).AddAssetSellOffer(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/rfqrpc.Rfq/AddAssetSellOffer",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RfqServer).AddAssetSellOffer(ctx, req.(*AddAssetSellOfferRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Rfq_QueryPeerAcceptedQuotes_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(QueryPeerAcceptedQuotesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RfqServer).QueryPeerAcceptedQuotes(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/rfqrpc.Rfq/QueryPeerAcceptedQuotes",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RfqServer).QueryPeerAcceptedQuotes(ctx, req.(*QueryPeerAcceptedQuotesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Rfq_SubscribeRfqEventNtfns_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(SubscribeRfqEventNtfnsRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(RfqServer).SubscribeRfqEventNtfns(m, &rfqSubscribeRfqEventNtfnsServer{stream})
}

type Rfq_SubscribeRfqEventNtfnsServer interface {
	Send(*RfqEvent) error
	grpc.ServerStream
}

type rfqSubscribeRfqEventNtfnsServer struct {
	grpc.ServerStream
}

func (x *rfqSubscribeRfqEventNtfnsServer) Send(m *RfqEvent) error {
	return x.ServerStream.SendMsg(m)
}

// Rfq_ServiceDesc is the grpc.ServiceDesc for Rfq service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Rfq_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "rfqrpc.Rfq",
	HandlerType: (*RfqServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "AddAssetBuyOrder",
			Handler:    _Rfq_AddAssetBuyOrder_Handler,
		},
		{
			MethodName: "AddAssetSellOrder",
			Handler:    _Rfq_AddAssetSellOrder_Handler,
		},
		{
			MethodName: "AddAssetSellOffer",
			Handler:    _Rfq_AddAssetSellOffer_Handler,
		},
		{
			MethodName: "QueryPeerAcceptedQuotes",
			Handler:    _Rfq_QueryPeerAcceptedQuotes_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "SubscribeRfqEventNtfns",
			Handler:       _Rfq_SubscribeRfqEventNtfns_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "rfqrpc/rfq.proto",
}
