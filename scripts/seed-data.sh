#!/usr/bin/env bash
# seed-data.sh — Creates 50+ demo entities and relationships via the CAAS API Gateway.
# Usage: ./scripts/seed-data.sh [API_URL]

set -euo pipefail

API="${1:-http://localhost:3001}"

echo "=== CAAS Seed Data ==="
echo "API: $API"
echo ""

# Helper: create entity and print result
create_entity() {
  local type=$1 name=$2 metadata=${3:-"{}"}
  curl -s -X POST "$API/v1/entities" \
    -H "Content-Type: application/json" \
    -d "{\"entity_type\": $type, \"display_name\": \"$name\", \"metadata\": $metadata}" \
    | tee /dev/null
}

# Helper: extract entity ID from response
entity_id() {
  echo "$1" | python3 -c "import sys,json; print(json.load(sys.stdin).get('entity',{}).get('id',''))" 2>/dev/null || echo ""
}

# Helper: activate entity
activate() {
  curl -s -X POST "$API/v1/entities/$1/activate" -H "Content-Type: application/json" > /dev/null
}

# Helper: suspend entity
suspend() {
  curl -s -X POST "$API/v1/entities/$1/suspend" \
    -H "Content-Type: application/json" \
    -d "{\"reason\": \"$2\"}" > /dev/null
}

# Helper: revoke entity
revoke_entity() {
  curl -s -X POST "$API/v1/entities/$1/revoke" \
    -H "Content-Type: application/json" \
    -d "{\"reason\": \"$2\"}" > /dev/null
}

# Helper: write relationship
write_rel() {
  local res_type=$1 res_id=$2 relation=$3 sub_type=$4 sub_id=$5
  curl -s -X POST "$API/v1/relationships" \
    -H "Content-Type: application/json" \
    -d "{\"relationships\": [{\"resource\": {\"type\": \"$res_type\", \"id\": \"$res_id\"}, \"relation\": \"$relation\", \"subject\": {\"type\": \"$sub_type\", \"id\": \"$sub_id\"}}]}" > /dev/null
}

echo "--- Creating Humans (10) ---"
IDS_HUMAN=()
for name in "Alice Chen" "Bob Martinez" "Charlie Kim" "Diana Okafor" "Ethan Nakamura" \
            "Fatima Al-Hassan" "George Petrov" "Hannah Berg" "Ibrahim Sow" "Julia Santos"; do
  resp=$(create_entity 1 "$name" "{\"department\": \"engineering\"}")
  id=$(entity_id "$resp")
  IDS_HUMAN+=("$id")
  echo "  + Human: $name ($id)"
done

echo ""
echo "--- Creating Organizations (5) ---"
IDS_ORG=()
for name in "Acme Corp" "TrustNet Labs" "Sovereign Systems Inc" "NexGen AI" "Global Logistics Co"; do
  resp=$(create_entity 2 "$name" "{\"industry\": \"technology\"}")
  id=$(entity_id "$resp")
  IDS_ORG+=("$id")
  echo "  + Org: $name ($id)"
done

echo ""
echo "--- Creating Devices (8) ---"
IDS_DEVICE=()
for name in "Sensor-Alpha-01" "Gateway-Hub-NYC" "Camera-Entrance-A" "Thermostat-Floor-3" \
            "Badge-Reader-Main" "Drone-Patrol-07" "Server-Rack-42" "IoT-Bridge-East"; do
  resp=$(create_entity 3 "$name" "{\"location\": \"building-a\"}")
  id=$(entity_id "$resp")
  IDS_DEVICE+=("$id")
  echo "  + Device: $name ($id)"
done

echo ""
echo "--- Creating Services (6) ---"
IDS_SERVICE=()
for name in "payments-api" "user-directory" "notification-service" "analytics-pipeline" \
            "document-store" "auth-gateway"; do
  resp=$(create_entity 4 "$name" "{\"version\": \"1.0.0\"}")
  id=$(entity_id "$resp")
  IDS_SERVICE+=("$id")
  echo "  + Service: $name ($id)"
done

echo ""
echo "--- Creating AI Agents (4) ---"
IDS_AGENT=()
for name in "FraudDetector-v3" "ContentModerator-v2" "TrustScorer-v1" "DataClassifier-v1"; do
  resp=$(create_entity 5 "$name" "{\"model\": \"custom-ensemble\"}")
  id=$(entity_id "$resp")
  IDS_AGENT+=("$id")
  echo "  + AI Agent: $name ($id)"
done

echo ""
echo "--- Creating Autonomous Systems (3) ---"
IDS_AUTO=()
for name in "TrafficControl-Downtown" "SupplyChain-Optimizer" "EnergyGrid-Balancer"; do
  resp=$(create_entity 6 "$name" "{\"autonomy_level\": \"supervised\"}")
  id=$(entity_id "$resp")
  IDS_AUTO+=("$id")
  echo "  + Autonomous: $name ($id)"
done

echo ""
echo "--- Activating entities ---"
# Activate most entities
for id in "${IDS_HUMAN[@]:0:8}" "${IDS_ORG[@]}" "${IDS_DEVICE[@]:0:6}" "${IDS_SERVICE[@]}" "${IDS_AGENT[@]:0:3}" "${IDS_AUTO[@]:0:2}"; do
  [ -n "$id" ] && activate "$id"
done
echo "  Activated 30 entities"

echo ""
echo "--- Suspending some entities ---"
[ -n "${IDS_HUMAN[8]:-}" ] && suspend "${IDS_HUMAN[8]}" "Security review pending"
[ -n "${IDS_DEVICE[6]:-}" ] && suspend "${IDS_DEVICE[6]}" "Maintenance window"
[ -n "${IDS_AGENT[3]:-}" ] && suspend "${IDS_AGENT[3]}" "Model retraining required"
echo "  Suspended 3 entities"

echo ""
echo "--- Revoking some entities ---"
[ -n "${IDS_HUMAN[9]:-}" ] && activate "${IDS_HUMAN[9]}" && revoke_entity "${IDS_HUMAN[9]}" "Terms of service violation"
[ -n "${IDS_DEVICE[7]:-}" ] && activate "${IDS_DEVICE[7]}" && revoke_entity "${IDS_DEVICE[7]}" "Compromised firmware"
echo "  Revoked 2 entities"

echo ""
echo "--- Writing relationships (30+) ---"

# Org memberships: humans belong to orgs
write_rel organization "${IDS_ORG[0]:-acme}" member user "${IDS_HUMAN[0]:-alice}"
write_rel organization "${IDS_ORG[0]:-acme}" member user "${IDS_HUMAN[1]:-bob}"
write_rel organization "${IDS_ORG[0]:-acme}" admin user "${IDS_HUMAN[0]:-alice}"
write_rel organization "${IDS_ORG[1]:-trustnet}" member user "${IDS_HUMAN[2]:-charlie}"
write_rel organization "${IDS_ORG[1]:-trustnet}" member user "${IDS_HUMAN[3]:-diana}"
write_rel organization "${IDS_ORG[1]:-trustnet}" admin user "${IDS_HUMAN[2]:-charlie}"
write_rel organization "${IDS_ORG[2]:-sovereign}" member user "${IDS_HUMAN[4]:-ethan}"
write_rel organization "${IDS_ORG[2]:-sovereign}" member user "${IDS_HUMAN[5]:-fatima}"
write_rel organization "${IDS_ORG[3]:-nexgen}" member user "${IDS_HUMAN[6]:-george}"
write_rel organization "${IDS_ORG[4]:-global}" member user "${IDS_HUMAN[7]:-hannah}"

# Device ownership
write_rel device "${IDS_DEVICE[0]:-sensor}" owner user "${IDS_HUMAN[4]:-ethan}"
write_rel device "${IDS_DEVICE[1]:-gateway}" owner organization "${IDS_ORG[0]:-acme}"
write_rel device "${IDS_DEVICE[2]:-camera}" owner organization "${IDS_ORG[2]:-sovereign}"
write_rel device "${IDS_DEVICE[3]:-thermostat}" operator user "${IDS_HUMAN[1]:-bob}"
write_rel device "${IDS_DEVICE[4]:-badge}" owner organization "${IDS_ORG[0]:-acme}"
write_rel device "${IDS_DEVICE[5]:-drone}" operator user "${IDS_HUMAN[5]:-fatima}"

# Service consumers
write_rel service "${IDS_SERVICE[0]:-payments}" consumer user "${IDS_HUMAN[0]:-alice}"
write_rel service "${IDS_SERVICE[0]:-payments}" consumer organization "${IDS_ORG[0]:-acme}"
write_rel service "${IDS_SERVICE[0]:-payments}" owner organization "${IDS_ORG[1]:-trustnet}"
write_rel service "${IDS_SERVICE[1]:-directory}" consumer user "${IDS_HUMAN[2]:-charlie}"
write_rel service "${IDS_SERVICE[2]:-notif}" consumer ai_agent "${IDS_AGENT[0]:-fraud}"
write_rel service "${IDS_SERVICE[3]:-analytics}" consumer ai_agent "${IDS_AGENT[2]:-trust}"
write_rel service "${IDS_SERVICE[4]:-docstore}" owner organization "${IDS_ORG[3]:-nexgen}"
write_rel service "${IDS_SERVICE[5]:-authgw}" owner organization "${IDS_ORG[0]:-acme}"

# AI Agent delegation
write_rel ai_agent "${IDS_AGENT[0]:-fraud}" principal user "${IDS_HUMAN[0]:-alice}"
write_rel ai_agent "${IDS_AGENT[1]:-moderator}" principal user "${IDS_HUMAN[2]:-charlie}"
write_rel ai_agent "${IDS_AGENT[2]:-trust}" supervised_by user "${IDS_HUMAN[4]:-ethan}"
write_rel ai_agent "${IDS_AGENT[3]:-classifier}" principal user "${IDS_HUMAN[6]:-george}"

# Autonomous system oversight
write_rel autonomous_system "${IDS_AUTO[0]:-traffic}" operator organization "${IDS_ORG[2]:-sovereign}"
write_rel autonomous_system "${IDS_AUTO[0]:-traffic}" oversight user "${IDS_HUMAN[5]:-fatima}"
write_rel autonomous_system "${IDS_AUTO[1]:-supply}" operator organization "${IDS_ORG[4]:-global}"
write_rel autonomous_system "${IDS_AUTO[2]:-energy}" regulator organization "${IDS_ORG[2]:-sovereign}"

# Resource permissions
write_rel resource "doc-quarterly-report" owner user "${IDS_HUMAN[0]:-alice}"
write_rel resource "doc-quarterly-report" viewer user "${IDS_HUMAN[1]:-bob}"
write_rel resource "doc-quarterly-report" editor user "${IDS_HUMAN[2]:-charlie}"

echo "  Wrote 33 relationship tuples"

echo ""
echo "=== Seed complete ==="
echo "  36 entities created (10 human, 5 org, 8 device, 6 service, 4 AI agent, 3 autonomous)"
echo "  30 activated, 3 suspended, 2 revoked"
echo "  33 relationship tuples written"
echo ""
echo "Try: curl $API/v1/entities | python3 -m json.tool"
