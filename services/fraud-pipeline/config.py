"""Configuration for the fraud pipeline service."""

import os


class Config:
    # gRPC server
    FRAUD_PORT = int(os.environ.get("FRAUD_PORT", "50054"))

    # Kafka / Redpanda
    KAFKA_BROKERS = os.environ.get("KAFKA_BROKERS", "localhost:19092")
    KAFKA_GROUP_ID = os.environ.get("KAFKA_GROUP_ID", "fraud-pipeline")
    KAFKA_TOPICS = [
        "caas.authz.events",
        "caas.entity.events",
        "caas.trust.events",
    ]

    # PostgreSQL
    DATABASE_URL = os.environ.get(
        "DATABASE_URL", "postgres://caas:caas_dev@localhost:5432/caas"
    )

    # gRPC endpoints for containment actions
    AUTHZ_ENDPOINT = os.environ.get("AUTHZ_ENDPOINT", "localhost:50051")
    ENTITY_ENDPOINT = os.environ.get("ENTITY_ENDPOINT", "localhost:50052")

    # Detection thresholds
    VELOCITY_WINDOW_SECONDS = 60
    VELOCITY_THRESHOLD = 20  # max events per entity in window
    GEO_ANOMALY_THRESHOLD = 0.8
    ISOLATION_FOREST_CONTAMINATION = 0.1
