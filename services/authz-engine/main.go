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
	port := envOr("AUTHZ_PORT", "50051")
	spicedbEndpoint := envOr("SPICEDB_ENDPOINT", "localhost:50051")
	spicedbKey := envOr("SPICEDB_PRESHARED_KEY", "caas_dev_key")
	redisURL := envOr("REDIS_URL", "redis://localhost:6379")
	kafkaBrokers := strings.Split(envOr("KAFKA_BROKERS", "localhost:19092"), ",")

	// Connect to SpiceDB
	spiceClient, err := NewSpiceDBClient(spicedbEndpoint, spicedbKey)
	if err != nil {
		log.Fatalf("Failed to connect to SpiceDB: %v", err)
	}

	// Connect to Redis for caching
	cache, err := NewRedisCache(redisURL)
	if err != nil {
		log.Printf("WARN: Redis unavailable, running without cache: %v", err)
		cache = nil
	}

	// Connect to Kafka/Redpanda for events
	events := NewEventProducer(kafkaBrokers, "caas.authz.events")
	defer func() {
		if events != nil {
			events.Close()
		}
	}()

	// Create authorization server
	authzServer := NewAuthorizationServer(spiceClient, cache, events)

	// Start gRPC server
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		log.Fatalf("Failed to listen on port %s: %v", port, err)
	}

	grpcServer := grpc.NewServer()
	caasv1.RegisterAuthorizationServiceServer(grpcServer, authzServer)

	// Health check
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("caas.v1.AuthorizationService", healthpb.HealthCheckResponse_SERVING)

	reflection.Register(grpcServer)

	// Graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down authz-engine...")
		cancel()
		grpcServer.GracefulStop()
	}()

	log.Printf("authz-engine listening on :%s (SpiceDB: %s)", port, spicedbEndpoint)

	_ = ctx
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
