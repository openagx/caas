"""
Fraud detection engine — Stage 1 (Detect) and Stage 2 (Classify).

Uses Isolation Forest for anomaly detection, velocity checks, and
pattern matching to identify fraud types.
"""

import time
from collections import defaultdict
from datetime import datetime, timezone

import numpy as np
from sklearn.ensemble import IsolationForest


class FraudDetector:
    """Multi-stage fraud detection with ML-based anomaly detection."""

    # Attack type classifications
    ATTACK_TYPES = {
        "account_takeover": {
            "severity": "critical",
            "indicators": ["rapid_permission_changes", "unusual_location", "credential_spray"],
        },
        "privilege_escalation": {
            "severity": "high",
            "indicators": ["admin_access_attempt", "role_change_burst", "unauthorized_resource"],
        },
        "sybil": {
            "severity": "high",
            "indicators": ["mass_entity_creation", "circular_endorsements", "bot_pattern"],
        },
        "exfiltration": {
            "severity": "critical",
            "indicators": ["bulk_read_operations", "unusual_export", "api_scraping"],
        },
        "impersonation": {
            "severity": "high",
            "indicators": ["did_mismatch", "device_change", "behavioral_shift"],
        },
    }

    def __init__(self, velocity_window: int = 60, velocity_threshold: int = 20):
        self.velocity_window = velocity_window
        self.velocity_threshold = velocity_threshold

        # Velocity tracking: entity_id -> list of timestamps
        self.event_timestamps: dict[str, list[float]] = defaultdict(list)

        # Isolation Forest for anomaly detection
        self.anomaly_model = IsolationForest(
            n_estimators=100,
            contamination=0.1,
            random_state=42,
        )
        # Pre-fit with synthetic normal patterns
        rng = np.random.RandomState(42)
        # Features: event_rate, unique_resources, permission_types, time_entropy, error_rate
        normal_data = np.column_stack([
            rng.poisson(5, 500),           # event_rate
            rng.poisson(3, 500),           # unique_resources
            rng.poisson(2, 500),           # permission_types
            rng.normal(0.7, 0.15, 500),    # time_entropy
            rng.beta(1, 20, 500),          # error_rate
        ])
        self.anomaly_model.fit(normal_data)

    def detect(self, event: dict) -> dict | None:
        """
        Stage 1: Detect anomalies in incoming events.
        Returns detection result or None if normal.
        Target: <50ms.
        """
        start = time.monotonic()

        entity_id = event.get("entity_id") or event.get("data", {}).get("entity_id", "")
        event_type = event.get("type", "")

        if not entity_id:
            return None

        # Velocity check
        now = time.time()
        self._record_event(entity_id, now)
        velocity = self._get_velocity(entity_id, now)

        # Feature extraction
        features = self._extract_features(entity_id, event, velocity)

        # ML anomaly detection
        prediction = self.anomaly_model.predict([features])[0]
        anomaly_score = -self.anomaly_model.score_samples([features])[0]

        is_anomalous = prediction == -1 or velocity > self.velocity_threshold

        latency_ms = int((time.monotonic() - start) * 1000)

        if not is_anomalous:
            return None

        return {
            "entity_id": entity_id,
            "event_type": event_type,
            "anomaly_score": float(anomaly_score),
            "velocity": velocity,
            "features": features.tolist(),
            "detection_latency_ms": latency_ms,
            "detected_at": datetime.now(timezone.utc),
        }

    def classify(self, detection: dict) -> dict:
        """
        Stage 2: Classify the type of attack.
        Returns classification with severity.
        Target: <100ms.
        """
        start = time.monotonic()

        event_type = detection.get("event_type", "")
        velocity = detection.get("velocity", 0)
        anomaly_score = detection.get("anomaly_score", 0)

        # Rule-based classification
        attack_type = "unknown"
        severity = "medium"

        if velocity > self.velocity_threshold * 2:
            if "check" in event_type or "permission" in event_type:
                attack_type = "account_takeover"
            elif "create" in event_type or "entity" in event_type:
                attack_type = "sybil"
            else:
                attack_type = "exfiltration"
        elif velocity > self.velocity_threshold:
            if "write" in event_type or "relationship" in event_type:
                attack_type = "privilege_escalation"
            else:
                attack_type = "impersonation"
        elif anomaly_score > 0.7:
            attack_type = "account_takeover"
        elif anomaly_score > 0.5:
            attack_type = "privilege_escalation"

        if attack_type in self.ATTACK_TYPES:
            severity = self.ATTACK_TYPES[attack_type]["severity"]

        latency_ms = int((time.monotonic() - start) * 1000)

        return {
            **detection,
            "attack_type": attack_type,
            "severity": severity,
            "classification_latency_ms": latency_ms,
        }

    def _record_event(self, entity_id: str, timestamp: float):
        self.event_timestamps[entity_id].append(timestamp)
        # Prune old events
        cutoff = timestamp - self.velocity_window
        self.event_timestamps[entity_id] = [
            t for t in self.event_timestamps[entity_id] if t > cutoff
        ]

    def _get_velocity(self, entity_id: str, now: float) -> int:
        cutoff = now - self.velocity_window
        return len([t for t in self.event_timestamps.get(entity_id, []) if t > cutoff])

    def _extract_features(self, entity_id: str, event: dict, velocity: int) -> np.ndarray:
        """Extract features for the anomaly model."""
        event_rate = velocity
        unique_resources = len(set(event.get("resources", []))) if "resources" in event else 1
        permission_types = 1
        # Time entropy: how spread out the events are (higher = more uniform)
        timestamps = self.event_timestamps.get(entity_id, [])
        if len(timestamps) > 1:
            diffs = np.diff(sorted(timestamps))
            if diffs.std() > 0:
                time_entropy = min(diffs.mean() / (diffs.std() + 0.001), 1.0)
            else:
                time_entropy = 0.1  # perfectly regular = suspicious
        else:
            time_entropy = 0.7

        error_rate = 0.0
        if event.get("data", {}).get("allowed") is False:
            error_rate = 0.5

        return np.array([event_rate, unique_resources, permission_types, time_entropy, error_rate])
