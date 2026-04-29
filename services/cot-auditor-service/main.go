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
	port := envOr("COT_AUDITOR_PORT", "50061")

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	caasv1.RegisterCoTAuditorServiceServer(grpcServer, &cotAuditorServer{})

	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("caas.v1.CoTAuditorService", healthpb.HealthCheckResponse_SERVING)

	reflection.Register(grpcServer)
	log.Printf("cot-auditor-service listening on :%s", port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func envOr(k, f string) string { return os.Getenv(k) }

type cotAuditorServer struct{ caasv1.UnimplementedCoTAuditorServiceServer }