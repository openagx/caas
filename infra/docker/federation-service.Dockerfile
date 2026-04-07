FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.work go.work.sum ./
COPY gen/go/ gen/go/
COPY packages/common-go/ packages/common-go/
COPY packages/sdk-go/ packages/sdk-go/
COPY services/federation-service/ services/federation-service/
# Copy stub modules so go.work resolves
COPY services/authz-engine/go.mod services/authz-engine/go.sum services/authz-engine/
COPY services/entity-service/go.mod services/entity-service/go.sum services/entity-service/
COPY services/trust-engine/go.mod services/trust-engine/go.sum services/trust-engine/
COPY services/decision-service/go.mod services/decision-service/go.sum services/decision-service/
COPY services/surplus-engine/go.mod services/surplus-engine/go.mod
WORKDIR /app/services/federation-service
RUN go mod download
RUN CGO_ENABLED=0 go build -o /federation-service .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /federation-service /usr/local/bin/federation-service
EXPOSE 50056
CMD ["federation-service"]
