FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.work go.work.sum ./
COPY gen/go/ gen/go/
COPY packages/common-go/ packages/common-go/
COPY packages/sdk-go/ packages/sdk-go/
COPY services/trust-engine/ services/trust-engine/
# Copy stub modules so go.work resolves
COPY services/authz-engine/go.mod services/authz-engine/go.sum services/authz-engine/
COPY services/entity-service/go.mod services/entity-service/go.sum services/entity-service/
COPY services/decision-service/go.mod services/decision-service/go.mod
COPY services/federation-service/go.mod services/federation-service/go.mod
COPY services/surplus-engine/go.mod services/surplus-engine/go.mod
WORKDIR /app/services/trust-engine
RUN go mod download
RUN CGO_ENABLED=0 go build -o /trust-engine .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /trust-engine /usr/local/bin/trust-engine
EXPOSE 50053
CMD ["trust-engine"]
