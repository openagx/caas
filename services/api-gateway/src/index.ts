import Fastify from "fastify";
import cors from "@fastify/cors";
import swagger from "@fastify/swagger";
import swaggerUi from "@fastify/swagger-ui";
import { createEntityClient, createAuthzClient, createTrustClient } from "./grpc-client.js";

const PORT = parseInt(process.env.API_PORT || "3001", 10);
const ENTITY_ENDPOINT = process.env.ENTITY_ENDPOINT || "localhost:50052";
const AUTHZ_ENDPOINT = process.env.AUTHZ_ENDPOINT || "localhost:50051";
const TRUST_ENDPOINT = process.env.TRUST_ENDPOINT || "localhost:50053";
const FRAUD_URL = process.env.FRAUD_URL || "http://localhost:50054";

const app = Fastify({ logger: true });

// gRPC clients
const entityClient = createEntityClient(ENTITY_ENDPOINT);
const authzClient = createAuthzClient(AUTHZ_ENDPOINT);
const trustClient = createTrustClient(TRUST_ENDPOINT);

// HTTP proxy helper for fraud-pipeline (REST-based service)
async function fraudFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${FRAUD_URL}${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", ...init?.headers },
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`Fraud service error ${res.status}: ${text}`);
  }
  return res.json() as Promise<T>;
}

// --- Plugins ---

await app.register(cors, { origin: true });

await app.register(swagger, {
  openapi: {
    info: {
      title: "CAAS API",
      description: "Continuous Autonomous Authorization System — REST Gateway",
      version: "0.1.0",
    },
    servers: [{ url: `http://localhost:${PORT}` }],
    tags: [
      { name: "entities", description: "Entity lifecycle management" },
      { name: "authorization", description: "Permission checks and lookups" },
      { name: "relationships", description: "Relationship tuple management" },
      { name: "trust", description: "Trust scoring, endorsements, and graph" },
      { name: "fraud", description: "Fraud detection, incidents, and simulation" },
    ],
  },
});

await app.register(swaggerUi, { routePrefix: "/docs" });

// --- Health ---

app.get("/health", async () => ({ status: "ok" }));

// --- Entity Routes ---

app.post<{ Body: { entity_type: number; display_name: string; metadata?: Record<string, unknown> } }>(
  "/v1/entities",
  {
    schema: {
      tags: ["entities"],
      body: {
        type: "object",
        required: ["entity_type", "display_name"],
        properties: {
          entity_type: { type: "integer", minimum: 1, maximum: 6 },
          display_name: { type: "string" },
          metadata: { type: "object", additionalProperties: true },
        },
      },
    },
  },
  async (req, reply) => {
    const { entity_type, display_name, metadata } = req.body;
    const result = await entityClient.createEntity({
      entityType: entity_type,
      displayName: display_name,
      metadata: metadata || undefined,
    });
    reply.code(201).send(result);
  }
);

app.get<{ Params: { id: string } }>(
  "/v1/entities/:id",
  {
    schema: {
      tags: ["entities"],
      params: {
        type: "object",
        properties: { id: { type: "string" } },
      },
    },
  },
  async (req) => {
    return entityClient.getEntity({ id: req.params.id });
  }
);

app.patch<{ Params: { id: string }; Body: { display_name?: string; metadata?: Record<string, unknown> } }>(
  "/v1/entities/:id",
  {
    schema: {
      tags: ["entities"],
      params: {
        type: "object",
        properties: { id: { type: "string" } },
      },
      body: {
        type: "object",
        properties: {
          display_name: { type: "string" },
          metadata: { type: "object", additionalProperties: true },
        },
      },
    },
  },
  async (req) => {
    return entityClient.updateEntity({
      id: req.params.id,
      displayName: req.body.display_name,
      metadata: req.body.metadata || undefined,
    });
  }
);

app.get<{ Querystring: { entity_type?: number; lifecycle_state?: number; page_size?: number; page_token?: string } }>(
  "/v1/entities",
  {
    schema: {
      tags: ["entities"],
      querystring: {
        type: "object",
        properties: {
          entity_type: { type: "integer" },
          lifecycle_state: { type: "integer" },
          page_size: { type: "integer", default: 50 },
          page_token: { type: "string" },
        },
      },
    },
  },
  async (req) => {
    return entityClient.listEntities({
      entityType: req.query.entity_type || 0,
      lifecycleState: req.query.lifecycle_state || 0,
      pageSize: req.query.page_size || 50,
      pageToken: req.query.page_token || "",
    });
  }
);

app.post<{ Params: { id: string } }>(
  "/v1/entities/:id/activate",
  {
    schema: {
      tags: ["entities"],
      params: {
        type: "object",
        properties: { id: { type: "string" } },
      },
    },
  },
  async (req) => {
    return entityClient.activateEntity({ id: req.params.id });
  }
);

app.post<{ Params: { id: string }; Body: { reason?: string } }>(
  "/v1/entities/:id/suspend",
  {
    schema: {
      tags: ["entities"],
      params: {
        type: "object",
        properties: { id: { type: "string" } },
      },
      body: {
        type: "object",
        properties: { reason: { type: "string" } },
      },
    },
  },
  async (req) => {
    return entityClient.suspendEntity({
      id: req.params.id,
      reason: req.body?.reason || "",
    });
  }
);

app.post<{ Params: { id: string }; Body: { reason?: string } }>(
  "/v1/entities/:id/revoke",
  {
    schema: {
      tags: ["entities"],
      params: {
        type: "object",
        properties: { id: { type: "string" } },
      },
      body: {
        type: "object",
        properties: { reason: { type: "string" } },
      },
    },
  },
  async (req) => {
    return entityClient.revokeEntity({
      id: req.params.id,
      reason: req.body?.reason || "",
    });
  }
);

// --- Authorization Routes ---

app.post<{
  Body: {
    subject: { type: string; id: string };
    permission: string;
    resource: { type: string; id: string };
    context?: Record<string, unknown>;
  };
}>(
  "/v1/authz/check",
  {
    schema: {
      tags: ["authorization"],
      body: {
        type: "object",
        required: ["subject", "permission", "resource"],
        properties: {
          subject: {
            type: "object",
            required: ["type", "id"],
            properties: {
              type: { type: "string" },
              id: { type: "string" },
            },
          },
          permission: { type: "string" },
          resource: {
            type: "object",
            required: ["type", "id"],
            properties: {
              type: { type: "string" },
              id: { type: "string" },
            },
          },
          context: { type: "object", additionalProperties: true },
        },
      },
    },
  },
  async (req) => {
    return authzClient.check({
      subject: req.body.subject,
      permission: req.body.permission,
      resource: req.body.resource,
      context: req.body.context || undefined,
    });
  }
);

app.post<{
  Body: {
    resource_type: string;
    permission: string;
    subject: { type: string; id: string };
  };
}>(
  "/v1/authz/lookup-subjects",
  {
    schema: {
      tags: ["authorization"],
      body: {
        type: "object",
        required: ["resource_type", "permission", "subject"],
        properties: {
          resource_type: { type: "string" },
          permission: { type: "string" },
          subject: {
            type: "object",
            required: ["type", "id"],
            properties: {
              type: { type: "string" },
              id: { type: "string" },
            },
          },
        },
      },
    },
  },
  async (req) => {
    // Note: LookupSubjects is a streaming RPC — the gRPC client here
    // uses the unary promisified wrapper which won't work for streaming.
    // For MVP, this returns an error; streaming support comes later.
    return { error: "streaming RPC not yet supported via REST gateway" };
  }
);

app.post<{
  Body: {
    resource_type: string;
    permission: string;
    subject: { type: string; id: string };
  };
}>(
  "/v1/authz/lookup-resources",
  {
    schema: {
      tags: ["authorization"],
      body: {
        type: "object",
        required: ["resource_type", "permission", "subject"],
        properties: {
          resource_type: { type: "string" },
          permission: { type: "string" },
          subject: {
            type: "object",
            required: ["type", "id"],
            properties: {
              type: { type: "string" },
              id: { type: "string" },
            },
          },
        },
      },
    },
  },
  async (req) => {
    return { error: "streaming RPC not yet supported via REST gateway" };
  }
);

// --- Relationship Routes ---

app.post<{
  Body: {
    relationships: Array<{
      resource: { type: string; id: string };
      relation: string;
      subject: { type: string; id: string };
    }>;
  };
}>(
  "/v1/relationships",
  {
    schema: {
      tags: ["relationships"],
      body: {
        type: "object",
        required: ["relationships"],
        properties: {
          relationships: {
            type: "array",
            items: {
              type: "object",
              required: ["resource", "relation", "subject"],
              properties: {
                resource: {
                  type: "object",
                  required: ["type", "id"],
                  properties: {
                    type: { type: "string" },
                    id: { type: "string" },
                  },
                },
                relation: { type: "string" },
                subject: {
                  type: "object",
                  required: ["type", "id"],
                  properties: {
                    type: { type: "string" },
                    id: { type: "string" },
                  },
                },
              },
            },
          },
        },
      },
    },
  },
  async (req, reply) => {
    const result = await authzClient.writeRelationships({
      relationships: req.body.relationships,
    });
    reply.code(201).send(result);
  }
);

app.get<{
  Querystring: {
    resource_type?: string;
    resource_id?: string;
    relation?: string;
    subject_type?: string;
    subject_id?: string;
  };
}>(
  "/v1/relationships",
  {
    schema: {
      tags: ["relationships"],
      querystring: {
        type: "object",
        properties: {
          resource_type: { type: "string" },
          resource_id: { type: "string" },
          relation: { type: "string" },
          subject_type: { type: "string" },
          subject_id: { type: "string" },
        },
      },
    },
  },
  async (req) => {
    return authzClient.readRelationships({
      resourceType: req.query.resource_type || "",
      resourceId: req.query.resource_id || "",
      relation: req.query.relation || "",
      subjectType: req.query.subject_type || "",
      subjectId: req.query.subject_id || "",
    });
  }
);

// --- Trust Routes ---

app.get<{ Params: { entityId: string } }>(
  "/v1/trust/:entityId",
  {
    schema: {
      tags: ["trust"],
      params: {
        type: "object",
        properties: { entityId: { type: "string" } },
      },
    },
  },
  async (req) => {
    return trustClient.getTrustScore({ entityId: req.params.entityId });
  }
);

app.get<{ Params: { entityId: string }; Querystring: { limit?: number } }>(
  "/v1/trust/:entityId/history",
  {
    schema: {
      tags: ["trust"],
      params: {
        type: "object",
        properties: { entityId: { type: "string" } },
      },
      querystring: {
        type: "object",
        properties: { limit: { type: "integer", default: 50 } },
      },
    },
  },
  async (req) => {
    return trustClient.getTrustHistory({
      entityId: req.params.entityId,
      limit: req.query.limit || 50,
    });
  }
);

app.get<{ Params: { entityId: string }; Querystring: { depth?: number } }>(
  "/v1/trust/:entityId/graph",
  {
    schema: {
      tags: ["trust"],
      params: {
        type: "object",
        properties: { entityId: { type: "string" } },
      },
      querystring: {
        type: "object",
        properties: { depth: { type: "integer", default: 2 } },
      },
    },
  },
  async (req) => {
    return trustClient.getTrustGraph({
      entityId: req.params.entityId,
      depth: req.query.depth || 2,
    });
  }
);

app.post<{
  Body: {
    endorser_id: string;
    endorsed_id: string;
    endorsement_type: string;
    weight?: number;
    evidence_hash?: string;
  };
}>(
  "/v1/trust/endorsements",
  {
    schema: {
      tags: ["trust"],
      body: {
        type: "object",
        required: ["endorser_id", "endorsed_id", "endorsement_type"],
        properties: {
          endorser_id: { type: "string" },
          endorsed_id: { type: "string" },
          endorsement_type: { type: "string" },
          weight: { type: "number", default: 1.0 },
          evidence_hash: { type: "string" },
        },
      },
    },
  },
  async (req, reply) => {
    const result = await trustClient.createEndorsement({
      endorserId: req.body.endorser_id,
      endorsedId: req.body.endorsed_id,
      endorsementType: req.body.endorsement_type,
      weight: req.body.weight || 1.0,
      evidenceHash: req.body.evidence_hash || "",
    });
    reply.code(201).send(result);
  }
);

app.delete<{ Params: { id: string } }>(
  "/v1/trust/endorsements/:id",
  {
    schema: {
      tags: ["trust"],
      params: {
        type: "object",
        properties: { id: { type: "string" } },
      },
    },
  },
  async (req) => {
    return trustClient.revokeEndorsement({ endorsementId: req.params.id });
  }
);

app.post<{
  Body: {
    entity_id: string;
    minimum_score?: number;
    minimum_dimensions?: Record<string, number>;
  };
}>(
  "/v1/trust/verify",
  {
    schema: {
      tags: ["trust"],
      body: {
        type: "object",
        required: ["entity_id"],
        properties: {
          entity_id: { type: "string" },
          minimum_score: { type: "integer" },
          minimum_dimensions: {
            type: "object",
            additionalProperties: { type: "integer" },
          },
        },
      },
    },
  },
  async (req) => {
    return trustClient.verifyTrust({
      entityId: req.body.entity_id,
      minimumScore: req.body.minimum_score || 0,
      minimumDimensions: req.body.minimum_dimensions || {},
    });
  }
);

// --- Fraud Routes (proxy to fraud-pipeline REST API) ---

app.get<{
  Querystring: {
    status?: string;
    min_severity?: string;
    affected_entity_id?: string;
    page_size?: number;
    page_token?: string;
  };
}>(
  "/v1/fraud/incidents",
  {
    schema: {
      tags: ["fraud"],
      querystring: {
        type: "object",
        properties: {
          status: { type: "string" },
          min_severity: { type: "string" },
          affected_entity_id: { type: "string" },
          page_size: { type: "integer", default: 50 },
          page_token: { type: "string" },
        },
      },
    },
  },
  async (req) => {
    const qs = new URLSearchParams();
    if (req.query.status) qs.set("status", req.query.status);
    if (req.query.min_severity) qs.set("min_severity", req.query.min_severity);
    if (req.query.affected_entity_id) qs.set("affected_entity_id", req.query.affected_entity_id);
    if (req.query.page_size) qs.set("page_size", String(req.query.page_size));
    if (req.query.page_token) qs.set("page_token", req.query.page_token);
    const query = qs.toString();
    return fraudFetch(`/v1/fraud/incidents${query ? `?${query}` : ""}`);
  }
);

app.get<{ Params: { id: string } }>(
  "/v1/fraud/incidents/:id",
  {
    schema: {
      tags: ["fraud"],
      params: {
        type: "object",
        properties: { id: { type: "string" } },
      },
    },
  },
  async (req) => {
    return fraudFetch(`/v1/fraud/incidents/${req.params.id}`);
  }
);

app.post<{
  Body: { attack_type: string; target_entity_id: string };
}>(
  "/v1/fraud/simulate",
  {
    schema: {
      tags: ["fraud"],
      body: {
        type: "object",
        required: ["attack_type", "target_entity_id"],
        properties: {
          attack_type: {
            type: "string",
            enum: ["account_takeover", "privilege_escalation", "sybil", "exfiltration", "impersonation"],
          },
          target_entity_id: { type: "string" },
        },
      },
    },
  },
  async (req) => {
    return fraudFetch("/v1/fraud/simulate", {
      method: "POST",
      body: JSON.stringify(req.body),
    });
  }
);

app.get(
  "/v1/fraud/metrics",
  {
    schema: { tags: ["fraud"] },
  },
  async () => {
    return fraudFetch("/v1/fraud/metrics");
  }
);

// --- Start ---

try {
  await app.listen({ port: PORT, host: "0.0.0.0" });
  app.log.info(`CAAS API Gateway running on http://localhost:${PORT}`);
  app.log.info(`Swagger docs at http://localhost:${PORT}/docs`);
} catch (err) {
  app.log.error(err);
  process.exit(1);
}
