module github.com/OpenAGX/caas/services/authz-engine

go 1.25.5

require (
	github.com/OpenAGX/caas/gen/go v0.0.0
	github.com/authzed/authzed-go v1.8.0
	github.com/authzed/grpcutil v0.0.0-20260105210157-e237581949c2
	github.com/redis/go-redis/v9 v9.18.0
	github.com/twmb/franz-go v1.20.7
	google.golang.org/grpc v1.80.0
)

require (
	buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go v1.36.11-20251209175733-2a1774d88802.1 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/certifi/gocertifi v0.0.0-20210507211836-431795d63e8d // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/envoyproxy/protoc-gen-validate v1.3.0 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.7 // indirect
	github.com/jzelinskie/stringz v0.0.3 // indirect
	github.com/klauspost/compress v1.18.4 // indirect
	github.com/pierrec/lz4/v4 v4.1.25 // indirect
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	github.com/twmb/franz-go/pkg/kmsg v1.12.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/net v0.49.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/text v0.33.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260128011058-8636f8732409 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260128011058-8636f8732409 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/OpenAGX/caas/gen/go => ../../gen/go
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20250303144028-a0af3efb3deb
)
