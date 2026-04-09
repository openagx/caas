FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.work go.work.sum ./
COPY gen/go/ gen/go/
COPY packages/common-go/ packages/common-go/
COPY packages/sdk-go/ packages/sdk-go/
COPY services/entity-service/ services/entity-service/
# Copy stub modules so go.work resolves
COPY services/authz-engine/go.mod services/authz-engine/go.mod
COPY services/authz-engine/go.sum services/authz-engine/go.sum
COPY services/trust-engine/go.mod services/trust-engine/go.mod
COPY services/trust-engine/go.sum services/trust-engine/go.sum
COPY services/decision-service/go.mod services/decision-service/go.mod
COPY services/decision-service/go.sum services/decision-service/go.sum
COPY services/federation-service/go.mod services/federation-service/go.mod
COPY services/federation-service/go.sum services/federation-service/go.sum
COPY services/surplus-engine/go.mod services/surplus-engine/go.mod
COPY services/surplus-engine/go.sum services/surplus-engine/go.sum
COPY services/did-service/go.mod services/did-service/go.mod
COPY services/did-service/go.sum services/did-service/go.sum
WORKDIR /app/services/entity-service
RUN go mod download
RUN CGO_ENABLED=0 go build -o /entity-service .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /entity-service /usr/local/bin/entity-service
EXPOSE 50052
CMD ["entity-service"]
