package conn

import (
	"context"
	"crypto/tls"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const (
	Endpoint = "api.drival.ai:443"
)

type clientOptions struct {
	target string // gRPC server address
	conn   *grpc.ClientConn
	svc    string // our target service
}

type ClientOption interface {
	apply(*clientOptions)
}

// fnClientOption wraps a function that modifies clientOptions into an
// implementation of the ClientOption interface.
type fnClientOption struct {
	f func(*clientOptions)
}

func (o *fnClientOption) apply(do *clientOptions) { o.f(do) }

func newFnClientOption(f func(*clientOptions)) *fnClientOption {
	return &fnClientOption{f: f}
}

func WithTarget(s string) ClientOption {
	return newFnClientOption(func(o *clientOptions) {
		o.target = s
	})
}

func WithTargetService(s string) ClientOption {
	return newFnClientOption(func(o *clientOptions) {
		o.svc = s
	})
}

func WithGrpcConnection(v *grpc.ClientConn) ClientOption {
	return newFnClientOption(func(o *clientOptions) {
		o.conn = v
	})
}

type GrpcClientConn struct {
	*grpc.ClientConn
	opts clientOptions
}

// Close closes the underlying connection.
func (c *GrpcClientConn) Close() {
	if c.opts.conn != nil {
		c.opts.conn.Close()
	}
}

// New returns a grpc connection to a Blue API target service.
func New(ctx context.Context, opts ...ClientOption) (*GrpcClientConn, error) {
	co := clientOptions{
		target: Endpoint,
	}

	for _, opt := range opts {
		opt.apply(&co)
	}

	if co.conn == nil {
		var gopts []grpc.DialOption
		if strings.Contains(co.target, "localhost") {
			gopts = append(gopts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		} else {
			creds := credentials.NewTLS(&tls.Config{})
			gopts = append(gopts, grpc.WithTransportCredentials(creds))
			gopts = append(gopts, grpc.WithPerRPCCredentials(NewRpcCredentials()))
		}

		var err error

		if co.svc != "" {
			gopts = append(gopts, grpc.WithUnaryInterceptor(func(ctx context.Context,
				method string, req interface{}, reply interface{}, cc *grpc.ClientConn,
				invoker grpc.UnaryInvoker, opts ...grpc.CallOption,
			) error {
				ctx = metadata.AppendToOutgoingContext(ctx, "service-name", co.svc)
				ctx = metadata.AppendToOutgoingContext(ctx, "x-agent", "vortex-go")
				return invoker(ctx, method, req, reply, cc, opts...)
			}))

			gopts = append(gopts, grpc.WithStreamInterceptor(func(ctx context.Context,
				desc *grpc.StreamDesc, cc *grpc.ClientConn, method string,
				streamer grpc.Streamer, opts ...grpc.CallOption,
			) (grpc.ClientStream, error) {
				ctx = metadata.AppendToOutgoingContext(ctx, "service-name", co.svc)
				ctx = metadata.AppendToOutgoingContext(ctx, "x-agent", "vortex-go")
				return streamer(ctx, desc, cc, method, opts...)
			}))
		}

		co.conn, err = grpc.NewClient(co.target, gopts...)
		if err != nil {
			return nil, err
		}
	}

	return &GrpcClientConn{co.conn, co}, nil
}
