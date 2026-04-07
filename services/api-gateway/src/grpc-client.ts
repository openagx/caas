import * as grpc from "@grpc/grpc-js";
import * as protoLoader from "@grpc/proto-loader";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const PROTO_DIR = path.resolve(__dirname, "../../../proto");

function loadProto(protoFile: string) {
  const packageDef = protoLoader.loadSync(
    path.join(PROTO_DIR, protoFile),
    {
      keepCase: false,
      longs: String,
      enums: String,
      defaults: true,
      oneofs: true,
      includeDirs: [PROTO_DIR],
    }
  );
  return grpc.loadPackageDefinition(packageDef);
}

function promisify(client: any, method: string) {
  return (request: any): Promise<any> =>
    new Promise((resolve, reject) => {
      client[method](request, (err: any, response: any) => {
        if (err) reject(err);
        else resolve(response);
      });
    });
}

// Entity Service client
export function createEntityClient(endpoint: string) {
  const proto = loadProto("caas/v1/entity.proto") as any;
  const EntityService = proto.caas.v1.EntityService;
  const client = new EntityService(
    endpoint,
    grpc.credentials.createInsecure()
  );
  return {
    createEntity: promisify(client, "createEntity"),
    getEntity: promisify(client, "getEntity"),
    updateEntity: promisify(client, "updateEntity"),
    listEntities: promisify(client, "listEntities"),
    activateEntity: promisify(client, "activateEntity"),
    suspendEntity: promisify(client, "suspendEntity"),
    revokeEntity: promisify(client, "revokeEntity"),
  };
}

// Trust Service client
export function createTrustClient(endpoint: string) {
  const proto = loadProto("caas/v1/trust.proto") as any;
  const TrustService = proto.caas.v1.TrustService;
  const client = new TrustService(
    endpoint,
    grpc.credentials.createInsecure()
  );
  return {
    getTrustScore: promisify(client, "getTrustScore"),
    getTrustHistory: promisify(client, "getTrustHistory"),
    createEndorsement: promisify(client, "createEndorsement"),
    revokeEndorsement: promisify(client, "revokeEndorsement"),
    getTrustGraph: promisify(client, "getTrustGraph"),
    verifyTrust: promisify(client, "verifyTrust"),
    attestTrust: promisify(client, "attestTrust"),
  };
}

// Decision Service client
export function createDecisionClient(endpoint: string) {
  const proto = loadProto("caas/v1/decision.proto") as any;
  const DecisionService = proto.caas.v1.DecisionService;
  const client = new DecisionService(
    endpoint,
    grpc.credentials.createInsecure()
  );
  return {
    createWorkflow: promisify(client, "createWorkflow"),
    getWorkflow: promisify(client, "getWorkflow"),
    listWorkflows: promisify(client, "listWorkflows"),
    submitVote: promisify(client, "submitVote"),
    getBlindCase: promisify(client, "getBlindCase"),
    getReviewerIntegrity: promisify(client, "getReviewerIntegrity"),
  };
}

// Federation Service client
export function createFederationClient(endpoint: string) {
  const proto = loadProto("caas/v1/federation.proto") as any;
  const FederationService = proto.caas.v1.FederationService;
  const client = new FederationService(
    endpoint,
    grpc.credentials.createInsecure()
  );
  return {
    registerNode: promisify(client, "registerNode"),
    getNode: promisify(client, "getNode"),
    listNodes: promisify(client, "listNodes"),
    updateNodeStatus: promisify(client, "updateNodeStatus"),
    proposeTreaty: promisify(client, "proposeTreaty"),
    acceptTreaty: promisify(client, "acceptTreaty"),
    getTreaty: promisify(client, "getTreaty"),
    listTreaties: promisify(client, "listTreaties"),
    terminateTreaty: promisify(client, "terminateTreaty"),
    federatedTrustQuery: promisify(client, "federatedTrustQuery"),
    federatedPermissionCheck: promisify(client, "federatedPermissionCheck"),
    createAuthorityOverride: promisify(client, "createAuthorityOverride"),
    getAuthorityOverride: promisify(client, "getAuthorityOverride"),
    listAuthorityOverrides: promisify(client, "listAuthorityOverrides"),
    issueCredential: promisify(client, "issueCredential"),
    verifyCredential: promisify(client, "verifyCredential"),
    revokeCredential: promisify(client, "revokeCredential"),
    listCredentials: promisify(client, "listCredentials"),
  };
}

// Surplus Service client
export function createSurplusClient(endpoint: string) {
  const proto = loadProto("caas/v1/surplus.proto") as any;
  const SurplusService = proto.caas.v1.SurplusService;
  const client = new SurplusService(
    endpoint,
    grpc.credentials.createInsecure()
  );
  return {
    createListing: promisify(client, "createListing"),
    getListing: promisify(client, "getListing"),
    listListings: promisify(client, "listListings"),
    updateListing: promisify(client, "updateListing"),
    cancelListing: promisify(client, "cancelListing"),
    findMatches: promisify(client, "findMatches"),
    acceptMatch: promisify(client, "acceptMatch"),
    updateMatchStatus: promisify(client, "updateMatchStatus"),
    getMatch: promisify(client, "getMatch"),
    listMatches: promisify(client, "listMatches"),
    verifyQuality: promisify(client, "verifyQuality"),
    getSurplusMetrics: promisify(client, "getSurplusMetrics"),
  };
}

// Authorization Service client
export function createAuthzClient(endpoint: string) {
  const proto = loadProto("caas/v1/authorization.proto") as any;
  const AuthorizationService = proto.caas.v1.AuthorizationService;
  const client = new AuthorizationService(
    endpoint,
    grpc.credentials.createInsecure()
  );
  return {
    check: promisify(client, "check"),
    writeRelationships: promisify(client, "writeRelationships"),
    readRelationships: promisify(client, "readRelationships"),
  };
}
