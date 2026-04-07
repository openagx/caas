"""PostgreSQL store for fraud incidents and metrics."""

import json
import uuid
from datetime import datetime, timezone

import psycopg


class FraudStore:
    def __init__(self, conninfo: str):
        self.conn = psycopg.connect(conninfo)
        self.conn.autocommit = True

    def close(self):
        self.conn.close()

    def create_incident(
        self,
        incident_type: str,
        severity: str,
        affected_entity_id: str,
        detection_timestamp: datetime,
        detection_latency_ms: int,
        evidence: dict,
    ) -> dict:
        incident_id = str(uuid.uuid4())
        now = datetime.now(timezone.utc)
        timeline = [
            {
                "stage": "detect",
                "action": f"Detected {incident_type}",
                "timestamp": detection_timestamp.isoformat(),
                "latency_ms": detection_latency_ms,
                "detail": f"Anomaly score triggered {incident_type} detection",
            }
        ]

        with self.conn.cursor() as cur:
            cur.execute(
                """INSERT INTO fraud_incidents
                   (id, incident_type, severity, status, affected_entity_id,
                    detection_timestamp, detection_latency_ms, evidence, timeline)
                   VALUES (%s, %s, %s, 'open', %s, %s, %s, %s, %s)""",
                (
                    incident_id,
                    incident_type,
                    severity,
                    affected_entity_id,
                    detection_timestamp,
                    detection_latency_ms,
                    json.dumps(evidence),
                    json.dumps(timeline),
                ),
            )

        return {
            "id": incident_id,
            "incident_type": incident_type,
            "severity": severity,
            "status": "open",
            "affected_entity_id": affected_entity_id,
            "detection_timestamp": detection_timestamp.isoformat(),
            "detection_latency_ms": detection_latency_ms,
            "evidence": evidence,
            "timeline": timeline,
            "created_at": now.isoformat(),
        }

    def update_incident_stage(
        self, incident_id: str, stage: str, action: str, latency_ms: int, detail: str
    ):
        now = datetime.now(timezone.utc)
        entry = {
            "stage": stage,
            "action": action,
            "timestamp": now.isoformat(),
            "latency_ms": latency_ms,
            "detail": detail,
        }

        with self.conn.cursor() as cur:
            cur.execute(
                """UPDATE fraud_incidents
                   SET timeline = timeline || %s::jsonb
                   WHERE id = %s""",
                (json.dumps([entry]), incident_id),
            )

    def contain_incident(self, incident_id: str, latency_ms: int):
        now = datetime.now(timezone.utc)
        with self.conn.cursor() as cur:
            cur.execute(
                """UPDATE fraud_incidents
                   SET status = 'contained', containment_timestamp = %s
                   WHERE id = %s""",
                (now, incident_id),
            )
        self.update_incident_stage(
            incident_id, "contain", "Entity contained", latency_ms, "Restrictive tuples added"
        )

    def get_incident(self, incident_id: str) -> dict | None:
        with self.conn.cursor() as cur:
            cur.execute(
                """SELECT id, incident_type, severity, status, affected_entity_id,
                          detection_timestamp, containment_timestamp, detection_latency_ms,
                          evidence, timeline, created_at
                   FROM fraud_incidents WHERE id = %s""",
                (incident_id,),
            )
            row = cur.fetchone()
            if not row:
                return None
            return self._row_to_dict(row)

    def list_incidents(
        self,
        status: str | None = None,
        min_severity: str | None = None,
        affected_entity_id: str | None = None,
        limit: int = 50,
        offset: int = 0,
    ) -> list[dict]:
        query = "SELECT id, incident_type, severity, status, affected_entity_id, detection_timestamp, containment_timestamp, detection_latency_ms, evidence, timeline, created_at FROM fraud_incidents WHERE 1=1"
        params: list = []

        if status:
            query += " AND status = %s"
            params.append(status)
        if min_severity:
            severity_order = {"low": 1, "medium": 2, "high": 3, "critical": 4}
            min_val = severity_order.get(min_severity, 0)
            valid = [s for s, v in severity_order.items() if v >= min_val]
            if valid:
                placeholders = ",".join(["%s"] * len(valid))
                query += f" AND severity IN ({placeholders})"
                params.extend(valid)
        if affected_entity_id:
            query += " AND affected_entity_id = %s"
            params.append(affected_entity_id)

        query += " ORDER BY created_at DESC LIMIT %s OFFSET %s"
        params.extend([limit, offset])

        with self.conn.cursor() as cur:
            cur.execute(query, params)
            return [self._row_to_dict(row) for row in cur.fetchall()]

    def get_metrics(self) -> dict:
        with self.conn.cursor() as cur:
            cur.execute(
                """SELECT
                     COUNT(*) as total,
                     COUNT(*) FILTER (WHERE status = 'open') as open_count,
                     AVG(detection_latency_ms) as avg_detection,
                     AVG(EXTRACT(EPOCH FROM (containment_timestamp - detection_timestamp)) * 1000)
                       FILTER (WHERE containment_timestamp IS NOT NULL) as avg_containment,
                     COUNT(*) FILTER (WHERE status = 'false_positive') as fp_count
                   FROM fraud_incidents"""
            )
            row = cur.fetchone()
            total = row[0] or 0
            return {
                "total_incidents": total,
                "open_incidents": row[1] or 0,
                "avg_detection_latency_ms": float(row[2] or 0),
                "avg_containment_latency_ms": float(row[3] or 0),
                "false_positive_count": row[4] or 0,
                "false_positive_rate": (row[4] or 0) / total if total > 0 else 0,
            }

    def _row_to_dict(self, row) -> dict:
        return {
            "id": str(row[0]),
            "incident_type": row[1],
            "severity": row[2],
            "status": row[3],
            "affected_entity_id": str(row[4]),
            "detection_timestamp": row[5].isoformat() if row[5] else None,
            "containment_timestamp": row[6].isoformat() if row[6] else None,
            "detection_latency_ms": row[7] or 0,
            "evidence": row[8] if isinstance(row[8], dict) else json.loads(row[8] or "{}"),
            "timeline": row[9] if isinstance(row[9], list) else json.loads(row[9] or "[]"),
            "created_at": row[10].isoformat() if row[10] else None,
        }
