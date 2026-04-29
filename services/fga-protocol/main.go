package main

// FGA Protocol Server
// Implements Fine-Grained Authorization API for client interoperability
// Ref: https://github.com/openfga/api

import (
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

func main() {
	port := envOr("FGA_PROTOCOL_PORT", "8081")
	
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	gs := grpc.NewServer()
	
	// Register FGA service (stub - implement with openfga/go-sdk)
	// RegisterFGAServiceServer(gs, &fgaServer{})
	
	hs := health.NewServer()
	healthpb.RegisterHealthServer(gs, hs)
	hs.SetServingStatus("openfga.v1.OpenFGAService", healthpb.HealthCheckResponse_SERVING)
	
	reflection.Register(gs)
	
	log.Printf("FGA protocol server listening on :%s", port)
	log.Printf("Endpoints: /stores/{id}/check, /stores/{id}/expand, /stores/{id}/list-objects")
	
	if err := gs.Serve(lis); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func envOr(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}

type fgaServer struct {
	UnimplementedOpenFGAServiceServer
}

// FGA Protocol endpoints:
// POST /stores/{store_id}/check          - Check authorization
// POST /stores/{store_id}/expand        - Expand subjects
// POST /stores/{store_id}/list-objects  - List authorized objects
// POST /stores/{store_id}/read          - Read tuples
// POST /stores/{store_id}/write         - Write/delete tuples
// POST /stores/{store_id}/read-authorization-model
// POST /stores/{store_id}/write-authorization-model

// TODO: Implement using openfga/go-sdk
// TODO: Add SpiceDB backend integration
// TODO: Add OPA sidecar for behavioral checks