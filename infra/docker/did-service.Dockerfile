FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.work go.work.sum ./
COPY gen/go/ gen/go/
COPY packages/common-go/ packages/common-go/
COPY packages/sdk-go/ packages/sdk-go/
COPY services/did-service/ services/did-service/
# Copy stub modules so go.work resolves
COPY services/authz-engine/go.mod services/authz-engine/go.sum services/authz-engine/
COPY services/entity-service/go.mod services/entity-service/go.sum services/entity-service/
COPY services/trust-engine/go.mod services/trust-engine/go.sum services/trust-engine/
COPY services/decision-service/go.mod services/decision-service/go.sum services/decision-service/
COPY services/federation-service/go.mod services/federation-service/go.sum services/federation-service/
COPY services/surplus-engine/go.mod services/surplus-engine/go.sum services/surplus-engine/
WORKDIR /app/services/did-service
RUN go mod download
RUN CGO_ENABLED=0 go build -o /did-service .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /did-service /usr/local/bin/did-service
EXPOSE 50058
CMD ["did-service"]
