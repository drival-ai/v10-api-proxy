package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/drival-ai/v10-go/iam/v1"

	"github.com/drival-ai/v10-go/base/v1"
	"github.com/golang/glog"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

var (
	gaddr = flag.String("gaddr", "localhost:8080", "The gRPC server address in the format of host:port")
	addr  = flag.String("addr", ":8081", "The address we serve this proxy in the format of host:port")
	local = flag.Bool("local", false, "If true, serve from local")
)

func main() {
	flag.Parse()
	defer glog.Flush()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	// Interrupt handler.
	go func() {
		sigch := make(chan os.Signal, 1)
		signal.Notify(sigch, syscall.SIGINT, syscall.SIGTERM)
		glog.Infof("signal: %v", <-sigch)
		cancel()
	}()

	var opts []grpc.DialOption
	if *local {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		// creds := credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})
		// opts = append(opts, grpc.WithTransportCredentials(creds))
	}

	opts = append(opts, grpc.WithUnaryInterceptor(func(ctx context.Context,
		method string, req interface{}, reply interface{}, cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker, opts ...grpc.CallOption,
	) error {
		ctx = metadata.AppendToOutgoingContext(ctx, "service-name", "v10-api")
		ctx = metadata.AppendToOutgoingContext(ctx, "x-grpc-gateway-proxy", "v10-api-proxy")
		return invoker(ctx, method, req, reply, cc, opts...)
	}))

	opts = append(opts, grpc.WithStreamInterceptor(func(ctx context.Context,
		desc *grpc.StreamDesc, cc *grpc.ClientConn, method string,
		streamer grpc.Streamer, opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		ctx = metadata.AppendToOutgoingContext(ctx, "service-name", "v10-api")
		ctx = metadata.AppendToOutgoingContext(ctx, "x-grpc-gateway-proxy", "v10-api-proxy")
		return streamer(ctx, desc, cc, method, opts...)
	}))

	// Create a client connection to our equivalent gRPC server.
	// This is where the grpc-gateway proxies the requests.
	conn, err := grpc.NewClient(*gaddr, opts...)
	if err != nil {
		glog.Fatal("NewClient failed:", err)
	}

	mux := runtime.NewServeMux(
		runtime.WithIncomingHeaderMatcher(func(key string) (string, bool) {
			switch strings.ToLower(key) {
			case "x-forwarded-for":
				return key, true
			default:
				return runtime.DefaultHeaderMatcher(key)
			}
		}),
	)

	err = iam.RegisterIamHandler(context.Background(), mux, conn)
	if err != nil {
		glog.Fatalf("RegisterCostHandler failed: %v", err)
	}
	err = base.RegisterV10Handler(context.Background(), mux, conn)
	if err != nil {
		glog.Fatalf("RegisterBaseHandler failed: %v", err)
	}

	trialForceCors := true // let's see if this works

	cors := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if correlationId := r.Header.Get("x-correlation-id"); correlationId != "" {
				w.Header().Set("x-correlation-id", correlationId)
			}

			if trialForceCors {
				w.Header().Set("Access-Control-Allow-Origin", "*")
				if r.Method == "OPTIONS" {
					headers := []string{"Content-Type", "Accept", "x-correlation-id"}
					w.Header().Set("Access-Control-Allow-Headers", strings.Join(headers, ","))
					methods := []string{"GET", "HEAD", "POST", "PUT", "DELETE"}
					w.Header().Set("Access-Control-Allow-Methods", strings.Join(methods, ","))
					glog.Infof("preflight request for %s", r.URL.Path)
					return
				}
			} else {
				if origin := r.Header.Get("Origin"); origin != "" {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					if r.Method == "OPTIONS" && r.Header.Get("Access-Control-Request-Method") != "" {
						headers := []string{"Content-Type", "Accept", "x-correlation-id"}
						w.Header().Set("Access-Control-Allow-Headers", strings.Join(headers, ","))
						methods := []string{"GET", "HEAD", "POST", "PUT", "DELETE"}
						w.Header().Set("Access-Control-Allow-Methods", strings.Join(methods, ","))
						glog.Infof("preflight request for %s", r.URL.Path)
						return
					}
				}
			}

			glog.Infof("[dbg] headers=%v", r.Header)
			glog.Infof("[dbg] method=%v, host=%v, url=%v", r.Method, r.URL.Host, r.URL)
			h.ServeHTTP(w, r)
		})
	}

	glog.Infof("serving grpc-gateway on %v", *addr)
	gw := &http.Server{Addr: *addr, Handler: cors(mux)}
	go gw.ListenAndServe() // run server

	go func() {
		<-ctx.Done()
		quit, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		done <- gw.Shutdown(quit)
	}()

	<-done
}
