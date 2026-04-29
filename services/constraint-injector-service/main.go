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
	port := envOr("CONSTRAINT_INJECTOR_PORT", "50062")
	lis, _ := net.Listen("tcp", fmt.Sprintf(":%s", port))
	grpcServer := grpc.NewServer()
	caasv1.RegisterConstraintInjectorServiceServer(grpcServer, &server{})
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("caas.v1.ConstraintInjectorService", healthpb.HealthCheckResponse_SERVING)
	reflection.Register(grpcServer)
	log.Printf("constraint-injector-service listening on :%s", port)
	grpcServer.Serve(lis)
}

func envOr(k, f string) string { return os.Getenv(k) }
type server struct{ caasv1.UnimplementedConstraintInjectorServiceServer }