package main

import (
	"fmt"
	"log"
	"net"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// SycophancyGuardService — detects constraint erosion from user pressure
func main() {
	port := envOr("SYCOPHANCY_GUARD_PORT", "50066")
	lis, _ := net.Listen("tcp", fmt.Sprintf(":%s", port))
	gs := grpc.NewServer()
	// caasv1.RegisterSycophancyGuardServiceServer(gs, &server{})
	hs := health.NewServer()
	healthpb.RegisterHealthServer(gs, hs)
	hs.SetServingStatus("caas.v1.SycophancyGuardService", healthpb.HealthCheckResponse_SERVING)
	reflection.Register(gs)
	log.Printf("sycophancy-guard-service: %s", port)
	gs.Serve(lis)
}
func envOr(k, f string) string { return os.Getenv(k) }
type server struct{}