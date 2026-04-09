FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.work go.work.sum ./
COPY gen/go/ gen/go/
COPY packages/common-go/ packages/common-go/
COPY packages/sdk-go/ packages/sdk-go/
COPY services/decision-service/ services/decision-service/
# Copy stub modules so go.work resolves
COPY services/authz-engine/go.mod services/authz-engine/go.sum services/authz-engine/
COPY services/entity-service/go.mod services/entity-service/go.sum services/entity-service/
COPY services/trust-engine/go.mod services/trust-engine/go.sum services/trust-engine/
COPY services/federation-service/go.mod services/federation-service/go.mod
COPY services/surplus-engine/go.mod services/surplus-engine/go.mod
COPY services/did-service/go.mod services/did-service/go.mod
WORKDIR /app/services/decision-service
RUN go mod download
RUN CGO_ENABLED=0 go build -o /decision-service .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /decision-service /usr/local/bin/decision-service
EXPOSE 50055
CMD ["decision-service"]
