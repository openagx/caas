"""
AAGFE Fraud Pipeline — main entry point.

Starts:
1. Kafka event consumer (background thread)
2. REST API server (FastAPI on FRAUD_PORT) implementing FraudService interface

The REST API is called by the API gateway. The Kafka consumer feeds
events into the fraud detection pipeline autonomously.
"""

import logging
import os
import signal
import sys

import uvicorn
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel

from config import Config
from consumer import EventConsumer
from detector import FraudDetector
from grpc_server import FraudServiceImpl
from kill_chain import KillChain
from store import FraudStore

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(name)s: %(message)s",
)
logger = logging.getLogger(__name__)

config = Config()
app = FastAPI(title="AAGFE Fraud Pipeline", version="0.1.0")

# Global instances (initialized on startup)
store: FraudStore | None = None
detector: FraudDetector | None = None
kill_chain_inst: KillChain | None = None
consumer: EventConsumer | None = None
service: FraudServiceImpl | None = None


# --- Request/Response Models ---


class SimulateRequest(BaseModel):
    attack_type: str
    target_entity_id: str


class IncidentQuery(BaseModel):
    status: str | None = None
    min_severity: str | None = None
    affected_entity_id: str | None = None
    page_size: int = 50
    page_token: str = ""


# --- Lifecycle ---


@app.on_event("startup")
async def startup():
    global store, detector, kill_chain_inst, consumer, service

    logger.info("Starting fraud-pipeline...")

    # PostgreSQL
    try:
        store = FraudStore(config.DATABASE_URL)
        logger.info("Connected to PostgreSQL")
    except Exception as e:
        logger.error("Failed to connect to PostgreSQL: %s", e)
        store = None

    # Detector
    detector = FraudDetector(
        velocity_window=config.VELOCITY_WINDOW_SECONDS,
        velocity_threshold=config.VELOCITY_THRESHOLD,
    )

    # Kill chain
    kill_chain_inst = KillChain(store, config) if store else None

    # Service
    service = FraudServiceImpl(store, detector, kill_chain_inst) if store and kill_chain_inst else None

    # Kafka consumer
    try:
        if store and detector and kill_chain_inst:
            consumer = EventConsumer(detector, kill_chain_inst, config)
            consumer.start()
            logger.info("Kafka consumer started")
    except Exception as e:
        logger.warning("Kafka unavailable, consumer disabled: %s", e)


@app.on_event("shutdown")
async def shutdown():
    if consumer:
        consumer.stop()
    if kill_chain_inst:
        kill_chain_inst.close()
    if store:
        store.close()
    logger.info("Fraud pipeline shut down")


# --- Endpoints ---


@app.get("/health")
async def health():
    return {
        "status": "ok",
        "store_connected": store is not None,
        "consumer_running": consumer is not None and consumer._running,
    }


@app.get("/v1/fraud/incidents")
async def list_incidents(
    status: str | None = None,
    min_severity: str | None = None,
    affected_entity_id: str | None = None,
    page_size: int = 50,
    page_token: str = "",
):
    if not service:
        raise HTTPException(503, "Fraud service not initialized")
    return service.list_incidents(status, min_severity, affected_entity_id, page_size, page_token)


@app.get("/v1/fraud/incidents/{incident_id}")
async def get_incident(incident_id: str):
    if not service:
        raise HTTPException(503, "Fraud service not initialized")
    incident = service.get_incident(incident_id)
    if not incident:
        raise HTTPException(404, f"Incident {incident_id} not found")
    return {"incident": incident}


@app.post("/v1/fraud/simulate")
async def simulate_attack(req: SimulateRequest):
    if not service:
        raise HTTPException(503, "Fraud service not initialized")
    valid_types = ["account_takeover", "privilege_escalation", "sybil", "exfiltration", "impersonation"]
    if req.attack_type not in valid_types:
        raise HTTPException(400, f"Invalid attack_type. Must be one of: {valid_types}")
    incident = service.simulate_attack(req.attack_type, req.target_entity_id)
    return {"incident": incident}


@app.get("/v1/fraud/metrics")
async def get_metrics():
    if not service:
        raise HTTPException(503, "Fraud service not initialized")
    return {"metrics": service.get_metrics()}


if __name__ == "__main__":
    port = config.FRAUD_PORT
    logger.info("Starting fraud-pipeline on port %d", port)
    uvicorn.run(app, host="0.0.0.0", port=port)
