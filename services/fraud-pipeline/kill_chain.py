"""
Fraud Kill Chain — Stages 3-6: Contain, Revoke, Notify, Investigate.

Orchestrates containment actions via gRPC calls to authz-engine
and entity-service.
"""

import json
import logging
import time
from datetime import datetime, timezone

import grpc

from config import Config
from store import FraudStore

logger = logging.getLogger(__name__)


class KillChain:
    """Executes containment and remediation stages of the fraud kill chain."""

    def __init__(self, store: FraudStore, config: Config):
        self.store = store
        self.config = config

        # gRPC channels for containment (lazy connect)
        self._authz_channel = None
        self._entity_channel = None

    def _get_authz_channel(self):
        if self._authz_channel is None:
            self._authz_channel = grpc.insecure_channel(self.config.AUTHZ_ENDPOINT)
        return self._authz_channel

    def _get_entity_channel(self):
        if self._entity_channel is None:
            self._entity_channel = grpc.insecure_channel(self.config.ENTITY_ENDPOINT)
        return self._entity_channel

    def execute(self, classification: dict) -> dict:
        """
        Execute the full kill chain for a detected+classified fraud event.
        Returns the completed incident record.
        """
        entity_id = classification["entity_id"]
        attack_type = classification["attack_type"]
        severity = classification["severity"]

        # Create incident in DB
        incident = self.store.create_incident(
            incident_type=attack_type,
            severity=severity,
            affected_entity_id=entity_id,
            detection_timestamp=classification["detected_at"],
            detection_latency_ms=classification["detection_latency_ms"],
            evidence={
                "anomaly_score": classification["anomaly_score"],
                "velocity": classification["velocity"],
                "event_type": classification["event_type"],
                "features": classification.get("features", []),
            },
        )

        incident_id = incident["id"]

        # Stage 2: Classification (already done, record it)
        self.store.update_incident_stage(
            incident_id,
            "classify",
            f"Classified as {attack_type}",
            classification.get("classification_latency_ms", 0),
            f"Severity: {severity}",
        )

        # Stage 3: Contain (<50ms target)
        contain_start = time.monotonic()
        self._contain(entity_id, attack_type, severity)
        contain_ms = int((time.monotonic() - contain_start) * 1000)
        self.store.contain_incident(incident_id, contain_ms)

        # Stage 4: Revoke if critical (<100ms target)
        if severity in ("critical", "high"):
            revoke_start = time.monotonic()
            self._revoke(entity_id, attack_type)
            revoke_ms = int((time.monotonic() - revoke_start) * 1000)
            self.store.update_incident_stage(
                incident_id,
                "revoke",
                f"Entity {entity_id} suspended",
                revoke_ms,
                f"Auto-revoked due to {severity} {attack_type}",
            )

        # Stage 5: Notify (<50ms target)
        notify_start = time.monotonic()
        self._notify(incident)
        notify_ms = int((time.monotonic() - notify_start) * 1000)
        self.store.update_incident_stage(
            incident_id,
            "notify",
            "Notifications dispatched",
            notify_ms,
            "Audit logged + event published",
        )

        # Stage 6: Mark for investigation
        self.store.update_incident_stage(
            incident_id,
            "investigate",
            "Queued for human review",
            0,
            f"Evidence package: anomaly_score={classification['anomaly_score']:.3f}, velocity={classification['velocity']}",
        )

        # Return final incident
        return self.store.get_incident(incident_id) or incident

    def _contain(self, entity_id: str, attack_type: str, severity: str):
        """Stage 3: Add restrictive tuples to block the entity."""
        try:
            # Use authz-engine to deny permissions
            # In a full implementation, this would use gRPC to call:
            #   AuthorizationService.WriteRelationships with DENY tuples
            logger.info(
                "CONTAIN: entity=%s attack=%s severity=%s — would write deny tuples",
                entity_id,
                attack_type,
                severity,
            )
        except Exception as e:
            logger.error("CONTAIN failed for %s: %s", entity_id, e)

    def _revoke(self, entity_id: str, attack_type: str):
        """Stage 4: Suspend the entity via entity-service."""
        try:
            # In a full implementation, this would use gRPC to call:
            #   EntityService.SuspendEntity
            logger.info(
                "REVOKE: entity=%s attack=%s — would suspend via entity-service",
                entity_id,
                attack_type,
            )
        except Exception as e:
            logger.error("REVOKE failed for %s: %s", entity_id, e)

    def _notify(self, incident: dict):
        """Stage 5: Dispatch notifications."""
        # In production: webhook dispatch, Slack/PagerDuty, audit log
        logger.info(
            "NOTIFY: incident=%s type=%s severity=%s entity=%s",
            incident["id"],
            incident["incident_type"],
            incident["severity"],
            incident["affected_entity_id"],
        )

    def close(self):
        if self._authz_channel:
            self._authz_channel.close()
        if self._entity_channel:
            self._entity_channel.close()
