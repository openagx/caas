"""
CAAS Trust ML Sidecar — FastAPI service for trust dimension scoring,
Sybil detection, and anomalous trust pattern detection.

Runs on port 8090, called by the trust-engine Go service.
"""

import hashlib
import os
import time

import numpy as np
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from sklearn.ensemble import IsolationForest

app = FastAPI(title="CAAS Trust ML Sidecar", version="0.1.0")

# --- Models ---

# Isolation Forest for anomaly detection (trained lazily)
_anomaly_model: IsolationForest | None = None
_sybil_model: IsolationForest | None = None


def get_anomaly_model() -> IsolationForest:
    """Lazy-init anomaly detection model."""
    global _anomaly_model
    if _anomaly_model is None:
        _anomaly_model = IsolationForest(
            n_estimators=100,
            contamination=0.1,
            random_state=42,
        )
        # Pre-fit with synthetic normal behavior
        rng = np.random.RandomState(42)
        normal_data = rng.normal(loc=0.6, scale=0.15, size=(500, 7))
        normal_data = np.clip(normal_data, 0, 1)
        _anomaly_model.fit(normal_data)
    return _anomaly_model


def get_sybil_model() -> IsolationForest:
    """Lazy-init Sybil detection model."""
    global _sybil_model
    if _sybil_model is None:
        _sybil_model = IsolationForest(
            n_estimators=100,
            contamination=0.15,
            random_state=123,
        )
        # Sybil features: endorsement_count, unique_endorsers, avg_weight,
        # time_since_creation, endorsement_velocity, reciprocal_ratio
        rng = np.random.RandomState(123)
        normal_data = np.column_stack([
            rng.poisson(10, 500),        # endorsement_count
            rng.poisson(8, 500),         # unique_endorsers
            rng.normal(0.7, 0.2, 500),   # avg_weight
            rng.exponential(30, 500),    # time_since_creation (days)
            rng.normal(0.3, 0.1, 500),   # endorsement_velocity
            rng.beta(2, 8, 500),         # reciprocal_ratio
        ])
        _sybil_model.fit(normal_data)
    return _sybil_model


# --- Request/Response Models ---


class ScoreRequest(BaseModel):
    entity_id: str


class ScoreResponse(BaseModel):
    identity_verification: int
    behavioral_consistency: int
    network_reputation: int
    transaction_history: int
    compliance_adherence: int
    temporal_stability: int
    peer_endorsement: int


class SybilRequest(BaseModel):
    entity_id: str
    endorsement_count: int = 0
    unique_endorsers: int = 0
    avg_weight: float = 0.5
    time_since_creation_days: float = 30.0
    endorsement_velocity: float = 0.3
    reciprocal_ratio: float = 0.1


class SybilResponse(BaseModel):
    sybil_probability: float
    is_anomalous: bool


class AnomalyRequest(BaseModel):
    entity_id: str
    dimensions: list[float]  # 7 normalized dimension values [0-1]


class AnomalyResponse(BaseModel):
    anomaly_score: float
    is_anomalous: bool


# --- Endpoints ---


@app.get("/health")
async def health():
    return {"status": "ok", "models": ["trust-scorer", "sybil-detector", "anomaly-detector"]}


@app.post("/score", response_model=ScoreResponse)
async def score_trust(req: ScoreRequest):
    """
    Score trust dimensions for an entity.

    For MVP, this uses a deterministic hash-based approach seeded by entity_id
    to generate consistent scores. In production, this would query behavioral
    data, transaction history, compliance records, etc. and run through
    trained models.
    """
    seed = int(hashlib.sha256(req.entity_id.encode()).hexdigest()[:8], 16)
    rng = np.random.RandomState(seed)

    # Generate scores within realistic ranges for each dimension
    dims = {
        "identity_verification": int(rng.normal(90, 30)),    # max 150
        "behavioral_consistency": int(rng.normal(120, 40)),  # max 200
        "network_reputation": int(rng.normal(85, 25)),       # max 150
        "transaction_history": int(rng.normal(80, 30)),      # max 150
        "compliance_adherence": int(rng.normal(60, 20)),     # max 100
        "temporal_stability": int(rng.normal(55, 18)),       # max 100
        "peer_endorsement": int(rng.normal(75, 25)),         # max 150
    }

    # Clamp to valid ranges
    maxes = {
        "identity_verification": 150,
        "behavioral_consistency": 200,
        "network_reputation": 150,
        "transaction_history": 150,
        "compliance_adherence": 100,
        "temporal_stability": 100,
        "peer_endorsement": 150,
    }

    for k in dims:
        dims[k] = max(0, min(dims[k], maxes[k]))

    return ScoreResponse(**dims)


@app.post("/sybil-detect", response_model=SybilResponse)
async def detect_sybil(req: SybilRequest):
    """
    Detect potential Sybil attack patterns.

    Uses Isolation Forest to identify entities with unusual endorsement
    patterns that may indicate coordinated fake accounts.
    """
    model = get_sybil_model()

    features = np.array([[
        req.endorsement_count,
        req.unique_endorsers,
        req.avg_weight,
        req.time_since_creation_days,
        req.endorsement_velocity,
        req.reciprocal_ratio,
    ]])

    # Isolation Forest: -1 = anomaly, 1 = normal
    prediction = model.predict(features)[0]
    anomaly_score = -model.score_samples(features)[0]

    # Normalize to [0, 1] probability
    sybil_prob = min(max(anomaly_score, 0), 1)

    return SybilResponse(
        sybil_probability=round(sybil_prob, 4),
        is_anomalous=prediction == -1,
    )


@app.post("/anomaly-detect", response_model=AnomalyResponse)
async def detect_anomaly(req: AnomalyRequest):
    """
    Detect anomalous trust dimension patterns.

    Uses Isolation Forest trained on normal trust patterns to identify
    entities with unusual dimension distributions.
    """
    if len(req.dimensions) != 7:
        raise HTTPException(status_code=400, detail="dimensions must have 7 values")

    model = get_anomaly_model()
    features = np.array([req.dimensions])

    prediction = model.predict(features)[0]
    anomaly_score = -model.score_samples(features)[0]

    return AnomalyResponse(
        anomaly_score=round(float(anomaly_score), 4),
        is_anomalous=prediction == -1,
    )


if __name__ == "__main__":
    import uvicorn

    port = int(os.environ.get("ML_SIDECAR_PORT", "8090"))
    uvicorn.run(app, host="0.0.0.0", port=port)
