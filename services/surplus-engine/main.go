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
	port := envOr("SURPLUS_PORT", "50057")
	dbURL := envOr("DATABASE_URL", "postgres://caas:caas_dev@localhost:5432/caas")
	kafkaBrokers := strings.Split(envOr("KAFKA_BROKERS", "localhost:19092"), ",")

	ctx := context.Background()

	store, err := NewPostgresStore(ctx, dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to PostgreSQL: %v", err)
	}
	defer store.Close()

	events := NewEventProducer(kafkaBrokers, "caas.surplus.events")
	defer func() {
		if events != nil {
			events.Close()
		}
	}()

	surplusServer := NewSurplusServer(store, events)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		log.Fatalf("Failed to listen on port %s: %v", port, err)
	}

	grpcServer := grpc.NewServer()
	caasv1.RegisterSurplusServiceServer(grpcServer, surplusServer)

	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("caas.v1.SurplusService", healthpb.HealthCheckResponse_SERVING)

	reflection.Register(grpcServer)

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down surplus-engine...")
		grpcServer.GracefulStop()
	}()

	log.Printf("surplus-engine listening on :%s", port)
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
