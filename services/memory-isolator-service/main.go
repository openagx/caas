package main

import (
	"fmt"
	"log"
	"net"
	"os"

	caasv1 "github.com/openagx/caas/gen/go/caas/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

func main() {
	port := envOr("MEMORY_ISOLATOR_PORT", "50064")
	lis, _ := net.Listen("tcp", fmt.Sprintf(":%s", port))
	grpcServer := grpc.NewServer()
	caasv1.RegisterMemoryIsolatorServiceServer(grpcServer, &server{})
	hs := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, hs)
	hs.SetServingStatus("caas.v1.MemoryIsolatorService", healthpb.HealthCheckResponse_SERVING)
	reflection.Register(grpcServer)
	log.Printf("memory-isolator-service: %s", port)
	grpcServer.Serve(lis)
}
func envOr(k, f string) string { return os.Getenv(k) }
type server struct{ caasv1.UnimplementedMemoryIsolatorServiceServer }