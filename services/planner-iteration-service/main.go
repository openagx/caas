package main

import (
	"fmt"
	"log"
	"net"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// PlannerIterationService — captures IAutoFunctionInvocationFilter signals
func main() {
	port := envOr("PLANNER_ITERATION_PORT", "50067")
	lis, _ := net.Listen("tcp", fmt.Sprintf(":%s", port))
	ps := grpc.NewServer()
	// caasv1.RegisterPlannerIterationServiceServer(ps, &server{})
	hs := health.NewServer()
	healthpb.RegisterHealthServer(ps, hs)
	hs.SetServingStatus("caas.v1.PlannerIterationService", healthpb.HealthCheckResponse_SERVING)
	reflection.Register(ps)
	log.Printf("planner-iteration-service: %s", port)
	ps.Serve(lis)
}
func envOr(k, f string) string { return os.Getenv(k) }
type server struct{}