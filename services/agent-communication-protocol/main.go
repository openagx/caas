package main

// Agent Communication Protocol (ACP) Service
// Agent-to-agent messaging: request/response, pub/sub, broadcast, streaming
// Port: 50070

import (
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

func main() {
	port := envOr("ACP_PORT", "50070")
	
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	gs := grpc.NewServer()
	// caasv1.RegisterAgentCommunicationProtocolServiceServer(gs, &acpServer{})

	hs := health.NewServer()
	healthpb.RegisterHealthServer(gs, hs)
	hs.SetServingStatus("caas.v1.AgentCommunicationProtocolService", healthpb.HealthCheckResponse_SERVING)

	reflection.Register(gs)

	log.Printf("Agent Communication Protocol (ACP) service listening on :%s", port)

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

type acpServer struct {
	UnimplementedAgentCommunicationProtocolServiceServer
	mu      sync.RWMutex
	inboxes map[string][]*IncomingMessage
	topics  map[string]map[string]bool
}

type IncomingMessage struct {
	MessageId   string
	FromAgentId string
	Topic       string
	Content     *MessageContent
}

// ACP endpoints:
// POST Send              - Direct message to agent
// POST Request           - Request/response
// POST Publish           - Pub/sub publish
// POST Subscribe         - Pub/sub subscribe  
// Stream Stream          - Bidirectional streaming
// POST Broadcast         - Broadcast to all
// POST GetInbox          - Get messages