module github.com/OpenAGX/caas/services/surplus-engine

go 1.25.5

require (
	github.com/OpenAGX/caas/gen/go v0.0.0
	github.com/jackc/pgx/v5 v5.7.5
	github.com/twmb/franz-go v1.20.7
	google.golang.org/grpc v1.80.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/klauspost/compress v1.18.4 // indirect
	github.com/pierrec/lz4/v4 v4.1.25 // indirect
	github.com/twmb/franz-go/pkg/kmsg v1.12.0 // indirect
	golang.org/x/crypto v0.48.0 // indirect
	golang.org/x/net v0.49.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260120221211-b8f7ae30c516 // indirect
)

replace (
	github.com/OpenAGX/caas/gen/go => ../../gen/go
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20250303144028-a0af3efb3deb
)
