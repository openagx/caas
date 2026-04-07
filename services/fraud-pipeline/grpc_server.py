"""
gRPC server implementing FraudServiceServer.

Since we don't generate Python stubs from proto for the MVP,
this uses grpc reflection and manual message construction.
We implement the service using a JSON-over-gRPC approach via
a simple HTTP/gRPC bridge, or more practically, we expose the
fraud service as a REST API that the API gateway calls directly.

For the MVP, we use a lightweight approach: the fraud-pipeline
exposes a REST API (FastAPI) on port 50054 that mirrors the
proto interface. The API gateway calls it via HTTP instead of gRPC.
"""

import logging
from datetime import datetime, timezone

from config import Config
from detector import FraudDetector
from kill_chain import KillChain
from store import FraudStore

logger = logging.getLogger(__name__)


class FraudServiceImpl:
    """Implements the FraudService proto interface (called via REST for MVP)."""

    def __init__(self, store: FraudStore, detector: FraudDetector, kill_chain: KillChain):
        self.store = store
        self.detector = detector
        self.kill_chain = kill_chain

    def list_incidents(
        self,
        status: str | None = None,
        min_severity: str | None = None,
        affected_entity_id: str | None = None,
        page_size: int = 50,
        page_token: str = "",
    ) -> dict:
        offset = int(page_token) if page_token else 0
        incidents = self.store.list_incidents(
            status=status,
            min_severity=min_severity,
            affected_entity_id=affected_entity_id,
            limit=page_size,
            offset=offset,
        )
        next_token = str(offset + page_size) if len(incidents) == page_size else ""
        return {"incidents": incidents, "next_page_token": next_token}

    def get_incident(self, incident_id: str) -> dict | None:
        return self.store.get_incident(incident_id)

    def simulate_attack(self, attack_type: str, target_entity_id: str) -> dict:
        """
        Simulate a fraud attack for demo purposes.
        Runs the full kill chain synchronously and returns the incident.
        """
        logger.info("SIMULATE: type=%s target=%s", attack_type, target_entity_id)

        # Create a synthetic detection
        now = datetime.now(timezone.utc)
        classification = {
            "entity_id": target_entity_id,
            "event_type": f"simulated_{attack_type}",
            "anomaly_score": 0.95,
            "velocity": 50,
            "features": [50, 10, 5, 0.1, 0.8],
            "detection_latency_ms": 12,
            "detected_at": now,
            "attack_type": attack_type,
            "severity": FraudDetector.ATTACK_TYPES.get(attack_type, {}).get("severity", "high"),
            "classification_latency_ms": 5,
        }

        # Execute kill chain
        incident = self.kill_chain.execute(classification)
        return incident

    def get_metrics(self) -> dict:
        return self.store.get_metrics()
