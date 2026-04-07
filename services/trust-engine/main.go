package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	caasv1 "github.com/OpenAGX/caas/gen/go/caas/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

func main() {
	port := envOr("TRUST_PORT", "50053")
	dbURL := envOr("DATABASE_URL", "postgres://caas:caas_dev@localhost:5432/caas")
	neo4jURI := envOr("NEO4J_URI", "bolt://localhost:7687")
	neo4jUser := envOr("NEO4J_USER", "neo4j")
	neo4jPassword := envOr("NEO4J_PASSWORD", "caas_dev_password")
	kafkaBrokers := strings.Split(envOr("KAFKA_BROKERS", "localhost:19092"), ",")
	mlSidecarURL := envOr("ML_SIDECAR_URL", "http://localhost:8090")

	ctx := context.Background()

	// Connect to PostgreSQL
	pgStore, err := NewPostgresStore(ctx, dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to PostgreSQL: %v", err)
	}
	defer pgStore.Close()

	// Connect to Neo4j
	graphStore, err := NewNeo4jStore(ctx, neo4jURI, neo4jUser, neo4jPassword)
	if err != nil {
		log.Fatalf("Failed to connect to Neo4j: %v", err)
	}
	defer graphStore.Close(ctx)

	// Connect to Kafka/Redpanda
	events := NewEventProducer(kafkaBrokers, "caas.trust.events")
	defer func() {
		if events != nil {
			events.Close()
		}
	}()

	// ML sidecar client
	mlClient := NewMLClient(mlSidecarURL)

	// Create trust server
	trustServer := NewTrustServer(pgStore, graphStore, events, mlClient)

	// Start gRPC server
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		log.Fatalf("Failed to listen on port %s: %v", port, err)
	}

	grpcServer := grpc.NewServer()
	caasv1.RegisterTrustServiceServer(grpcServer, trustServer)

	// Health check
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("caas.v1.TrustService", healthpb.HealthCheckResponse_SERVING)

	reflection.Register(grpcServer)

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down trust-engine...")
		grpcServer.GracefulStop()
	}()

	log.Printf("trust-engine listening on :%s", port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("gRPC server error: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
