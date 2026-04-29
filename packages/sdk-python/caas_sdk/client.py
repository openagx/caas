"""AAGFE API client."""

from __future__ import annotations

from typing import Any

import httpx

from .types import AuthorityTier, EntityType, VoteType


class CaasError(Exception):
    """Error from the AAGFE API."""

    def __init__(self, status_code: int, body: str, path: str):
        self.status_code = status_code
        self.body = body
        self.path = path
        super().__init__(f"AAGFE API error {status_code} on {path}: {body}")


class CaasClient:
    """Client for the AAGFE API Gateway.

    Usage::

        client = CaasClient("http://localhost:3001")
        entity = client.entities.create(EntityType.HUMAN, "Alice")
        result = client.authz.check(
            subject={"type": "user", "id": entity["id"]},
            permission="view",
            resource={"type": "document", "id": "doc-1"},
        )
    """

    def __init__(self, base_url: str, api_key: str | None = None, timeout: float = 30.0):
        self._base_url = base_url.rstrip("/")
        self._api_key = api_key
        self._client = httpx.Client(timeout=timeout)

        self.entities = _EntityClient(self)
        self.authz = _AuthzClient(self)
        self.trust = _TrustClient(self)
        self.fraud = _FraudClient(self)
        self.decisions = _DecisionClient(self)
        self.federation = _FederationClient(self)
        self.credentials = _CredentialClient(self)

    def _request(self, method: str, path: str, **kwargs: Any) -> Any:
        headers = {"Content-Type": "application/json"}
        if self._api_key:
            headers["Authorization"] = f"Bearer {self._api_key}"

        resp = self._client.request(method, f"{self._base_url}{path}", headers=headers, **kwargs)
        if resp.status_code >= 400:
            raise CaasError(resp.status_code, resp.text, path)
        return resp.json()

    def health(self) -> dict[str, str]:
        return self._request("GET", "/health")

    def close(self):
        self._client.close()

    def __enter__(self):
        return self

    def __exit__(self, *args: Any):
        self.close()


class _EntityClient:
    def __init__(self, client: CaasClient):
        self._c = client

    def create(self, entity_type: EntityType, display_name: str, metadata: dict | None = None) -> dict:
        return self._c._request("POST", "/v1/entities", json={
            "entity_type": int(entity_type),
            "display_name": display_name,
            "metadata": metadata or {},
        })

    def get(self, entity_id: str) -> dict:
        return self._c._request("GET", f"/v1/entities/{entity_id}")

    def list(self, page_size: int = 50) -> dict:
        return self._c._request("GET", f"/v1/entities?page_size={page_size}")

    def activate(self, entity_id: str) -> dict:
        return self._c._request("POST", f"/v1/entities/{entity_id}/activate")

    def suspend(self, entity_id: str, reason: str = "") -> dict:
        return self._c._request("POST", f"/v1/entities/{entity_id}/suspend", json={"reason": reason})

    def revoke(self, entity_id: str, reason: str = "") -> dict:
        return self._c._request("POST", f"/v1/entities/{entity_id}/revoke", json={"reason": reason})


class _AuthzClient:
    def __init__(self, client: CaasClient):
        self._c = client

    def check(self, subject: dict, permission: str, resource: dict, context: dict | None = None) -> dict:
        return self._c._request("POST", "/v1/authz/check", json={
            "subject": subject,
            "permission": permission,
            "resource": resource,
            "context": context,
        })

    def write_relationships(self, relationships: list[dict]) -> dict:
        return self._c._request("POST", "/v1/relationships", json={"relationships": relationships})

    def read_relationships(self, resource_type: str = "", relation: str = "") -> dict:
        params = {}
        if resource_type:
            params["resource_type"] = resource_type
        if relation:
            params["relation"] = relation
        qs = "&".join(f"{k}={v}" for k, v in params.items())
        return self._c._request("GET", f"/v1/relationships{'?' + qs if qs else ''}")


class _TrustClient:
    def __init__(self, client: CaasClient):
        self._c = client

    def get_score(self, entity_id: str) -> dict:
        return self._c._request("GET", f"/v1/trust/{entity_id}")

    def verify(self, entity_id: str, minimum_score: int = 0, minimum_dimensions: dict | None = None) -> dict:
        return self._c._request("POST", "/v1/trust/verify", json={
            "entity_id": entity_id,
            "minimum_score": minimum_score,
            "minimum_dimensions": minimum_dimensions or {},
        })

    def attest(self, entity_id: str, threshold: int) -> dict:
        """Privacy-preserving trust attestation. Proves entity meets threshold without revealing score."""
        return self._c._request("POST", "/v1/trust/attest", json={
            "entity_id": entity_id,
            "threshold": threshold,
        })

    def create_endorsement(self, endorser_id: str, endorsed_id: str, endorsement_type: str, weight: float = 1.0) -> dict:
        return self._c._request("POST", "/v1/trust/endorsements", json={
            "endorser_id": endorser_id,
            "endorsed_id": endorsed_id,
            "endorsement_type": endorsement_type,
            "weight": weight,
        })


class _FraudClient:
    def __init__(self, client: CaasClient):
        self._c = client

    def list_incidents(self, page_size: int = 50) -> dict:
        return self._c._request("GET", f"/v1/fraud/incidents?page_size={page_size}")

    def get_incident(self, incident_id: str) -> dict:
        return self._c._request("GET", f"/v1/fraud/incidents/{incident_id}")

    def simulate(self, attack_type: str, target_entity_id: str) -> dict:
        return self._c._request("POST", "/v1/fraud/simulate", json={
            "attack_type": attack_type,
            "target_entity_id": target_entity_id,
        })

    def get_metrics(self) -> dict:
        return self._c._request("GET", "/v1/fraud/metrics")


class _DecisionClient:
    def __init__(self, client: CaasClient):
        self._c = client

    def create_workflow(
        self,
        workflow_type: str,
        subject_entity_id: str,
        required_approvals: int = 2,
        total_reviewers: int = 3,
        blind_review: bool = True,
    ) -> dict:
        return self._c._request("POST", "/v1/decisions/workflows", json={
            "workflow_type": workflow_type,
            "subject_entity_id": subject_entity_id,
            "required_approvals": required_approvals,
            "total_reviewers": total_reviewers,
            "blind_review": blind_review,
        })

    def submit_vote(self, workflow_id: str, reviewer_id: str, vote: VoteType, reasoning: str = "") -> dict:
        return self._c._request("POST", "/v1/decisions/votes", json={
            "workflow_id": workflow_id,
            "reviewer_id": reviewer_id,
            "vote": int(vote),
            "reasoning": reasoning,
        })

    def get_reviewer_integrity(self, reviewer_id: str) -> dict:
        return self._c._request("GET", f"/v1/decisions/reviewers/{reviewer_id}/integrity")


class _FederationClient:
    def __init__(self, client: CaasClient):
        self._c = client

    def register_node(self, name: str, jurisdiction: str, endpoint: str) -> dict:
        return self._c._request("POST", "/v1/federation/nodes", json={
            "name": name,
            "jurisdiction": jurisdiction,
            "endpoint": endpoint,
        })

    def list_nodes(self) -> dict:
        return self._c._request("GET", "/v1/federation/nodes")

    def propose_treaty(self, target_node_id: str, trust_weight: float = 0.5, allowed_operations: list[str] | None = None) -> dict:
        return self._c._request("POST", "/v1/federation/treaties", json={
            "target_node_id": target_node_id,
            "trust_weight": trust_weight,
            "allowed_operations": allowed_operations or [],
        })

    def create_authority_override(
        self,
        workflow_id: str,
        authority_entity_id: str,
        tier: AuthorityTier,
        justification: str,
        outcome: str,
    ) -> dict:
        return self._c._request("POST", "/v1/authority/overrides", json={
            "workflow_id": workflow_id,
            "authority_entity_id": authority_entity_id,
            "tier": int(tier),
            "justification": justification,
            "outcome": outcome,
        })


class _CredentialClient:
    def __init__(self, client: CaasClient):
        self._c = client

    def issue(self, entity_id: str, credential_type: str, claims: dict | None = None) -> dict:
        return self._c._request("POST", "/v1/credentials", json={
            "entity_id": entity_id,
            "credential_type": credential_type,
            "claims": claims or {},
        })

    def verify(self, credential_id: str) -> dict:
        return self._c._request("GET", f"/v1/credentials/{credential_id}/verify")

    def revoke(self, credential_id: str, reason: str = "") -> dict:
        return self._c._request("POST", f"/v1/credentials/{credential_id}/revoke", json={"reason": reason})

    def list(self, entity_id: str = "", credential_type: str = "") -> dict:
        params = {}
        if entity_id:
            params["entity_id"] = entity_id
        if credential_type:
            params["credential_type"] = credential_type
        qs = "&".join(f"{k}={v}" for k, v in params.items())
        return self._c._request("GET", f"/v1/credentials{'?' + qs if qs else ''}")
