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
	port := envOr("INTENT_REGISTRY_PORT", "50060")

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	caasv1.RegisterIntentRegistryServiceServer(grpcServer, &intentRegistryServer{})

	// Health
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("caas.v1.IntentRegistryService", healthpb.HealthCheckResponse_SERVING)

	reflection.Register(grpcServer)

	log.Printf("intent-registry-service listening on :%s", port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// intentRegistryServer implements IntentRegistryServiceServer
type intentRegistryServer struct {
	caasv1.UnimplementedIntentRegistryServiceServer
}