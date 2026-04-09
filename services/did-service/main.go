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
	port := envOr("DID_PORT", "50058")
	dbURL := envOr("DATABASE_URL", "postgres://caas:caas_dev@localhost:5432/caas")
	kafkaBrokers := strings.Split(envOr("KAFKA_BROKERS", "localhost:19092"), ",")
	encKeyHex := envOr("DID_KEY_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	domain := envOr("DID_DOMAIN", "caas.local")

	encKey, err := ParseEncryptionKey(encKeyHex)
	if err != nil {
		log.Fatalf("Invalid DID_KEY_ENCRYPTION_KEY: %v", err)
	}

	ctx := context.Background()

	store, err := NewPostgresStore(ctx, dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to PostgreSQL: %v", err)
	}
	defer store.Close()

	events := NewEventProducer(kafkaBrokers, "caas.did.events")
	defer func() {
		if events != nil {
			events.Close()
		}
	}()

	didServer := NewDIDServer(store, events, encKey, domain)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		log.Fatalf("Failed to listen on port %s: %v", port, err)
	}

	grpcServer := grpc.NewServer()
	caasv1.RegisterDIDServiceServer(grpcServer, didServer)

	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("caas.v1.DIDService", healthpb.HealthCheckResponse_SERVING)

	reflection.Register(grpcServer)

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down did-service...")
		grpcServer.GracefulStop()
	}()

	log.Printf("did-service listening on :%s", port)
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
