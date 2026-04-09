package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	caasv1 "github.com/OpenAGX/caas/gen/go/caas/v1"
	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

const (
	configKeyFederationIssuerDID = "federation_issuer_did"
)

func main() {
	port := envOr("FEDERATION_PORT", "50056")
	dbURL := envOr("DATABASE_URL", "postgres://caas:caas_dev@localhost:5432/caas")
	kafkaBrokers := strings.Split(envOr("KAFKA_BROKERS", "localhost:19092"), ",")
	didEndpoint := envOr("DID_ENDPOINT", "did-service:50058")

	ctx := context.Background()

	store, err := NewPostgresStore(ctx, dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to PostgreSQL: %v", err)
	}
	defer store.Close()

	events := NewEventProducer(kafkaBrokers, "caas.federation.events")
	defer func() {
		if events != nil {
			events.Close()
		}
	}()

	// Dial did-service
	didConn, err := grpc.NewClient(didEndpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to create did-service client: %v", err)
	}
	defer didConn.Close()
	didClient := caasv1.NewDIDServiceClient(didConn)

	// Ensure federation issuer DID exists (bootstrap)
	issuerDID, err := ensureFederationIssuerDID(ctx, store, didClient)
	if err != nil {
		log.Fatalf("Failed to ensure federation issuer DID: %v", err)
	}
	log.Printf("federation issuer DID: %s", issuerDID)

	fedServer := NewFederationServer(store, events, didClient, issuerDID)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		log.Fatalf("Failed to listen on port %s: %v", port, err)
	}

	grpcServer := grpc.NewServer()
	caasv1.RegisterFederationServiceServer(grpcServer, fedServer)

	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("caas.v1.FederationService", healthpb.HealthCheckResponse_SERVING)

	reflection.Register(grpcServer)

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down federation-service...")
		grpcServer.GracefulStop()
	}()

	log.Printf("federation-service listening on :%s", port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("gRPC server error: %v", err)
	}
}

// ensureFederationIssuerDID returns the federation's issuer DID, creating one via
// did-service if none exists yet. The result is persisted in federation_config.
func ensureFederationIssuerDID(ctx context.Context, store *PostgresStore, didClient caasv1.DIDServiceClient) (string, error) {
	// 1. Try to load existing issuer DID from config
	existing, err := store.GetFederationConfig(ctx, configKeyFederationIssuerDID)
	if err == nil && existing != "" {
		return existing, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("load federation issuer DID from config: %w", err)
	}

	// 2. No issuer DID yet — create one via did-service (did:key, Ed25519)
	createCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := didClient.CreateDID(createCtx, &caasv1.CreateDIDRequest{
		Method:  caasv1.DIDMethod_DID_METHOD_KEY,
		KeyType: caasv1.KeyType_KEY_TYPE_ED25519,
	})
	if err != nil {
		return "", fmt.Errorf("create federation issuer DID via did-service: %w", err)
	}
	if resp.Document == nil || resp.Document.Id == "" {
		return "", fmt.Errorf("did-service returned empty DID document")
	}

	did := resp.Document.Id

	// 3. Persist in config for next boot
	if err := store.SetFederationConfig(ctx, configKeyFederationIssuerDID, did); err != nil {
		return "", fmt.Errorf("persist federation issuer DID: %w", err)
	}

	return did, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
