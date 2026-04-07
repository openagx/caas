import Fastify from "fastify";
import cors from "@fastify/cors";
import swagger from "@fastify/swagger";
import swaggerUi from "@fastify/swagger-ui";
import { createEntityClient, createAuthzClient } from "./grpc-client.js";

const PORT = parseInt(process.env.API_PORT || "3001", 10);
const ENTITY_ENDPOINT = process.env.ENTITY_ENDPOINT || "localhost:50052";
const AUTHZ_ENDPOINT = process.env.AUTHZ_ENDPOINT || "localhost:50051";

const app = Fastify({ logger: true });

// gRPC clients
const entityClient = createEntityClient(ENTITY_ENDPOINT);
const authzClient = createAuthzClient(AUTHZ_ENDPOINT);

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

// --- Start ---

try {
  await app.listen({ port: PORT, host: "0.0.0.0" });
  app.log.info(`CAAS API Gateway running on http://localhost:${PORT}`);
  app.log.info(`Swagger docs at http://localhost:${PORT}/docs`);
} catch (err) {
  app.log.error(err);
  process.exit(1);
}
