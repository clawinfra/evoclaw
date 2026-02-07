#!/bin/bash
# ═══════════════════════════════════════════════════════════
# EvoClaw E2E Flow Test
# Tests the full loop: Human → Orchestrator → Broker → Agent → back
# ═══════════════════════════════════════════════════════════

set -e
BROKER="localhost"
MQTT_PORT=1883
API="http://localhost:8420"
# Auto-detect an active agent (one with messages > 0, or fall back to first)
AGENT_ID=$(curl -s "$API/api/agents" 2>/dev/null | python3 -c "
import sys,json
agents = json.load(sys.stdin)
# Prefer agent with recent activity
active = [a for a in agents if a.get('message_count',0) > 0]
if active:
    # Find one that's actually a Pi agent (has raspberrypi in name)
    pi = [a for a in active if 'raspberrypi' in a['id']]
    print(pi[0]['id'] if pi else active[0]['id'])
elif agents:
    print(agents[0]['id'])
else:
    print('pi1-edge')
" 2>/dev/null)
echo "Using agent: $AGENT_ID"
PASS=0
FAIL=0
TOTAL=0

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

check() {
    TOTAL=$((TOTAL + 1))
    local name="$1"
    local result="$2"
    local expected="$3"
    
    if echo "$result" | grep -q "$expected"; then
        PASS=$((PASS + 1))
        echo -e "  ${GREEN}✅ PASS${NC} — $name"
    else
        FAIL=$((FAIL + 1))
        echo -e "  ${RED}❌ FAIL${NC} — $name"
        echo -e "    Expected: ${YELLOW}$expected${NC}"
        echo -e "    Got: ${YELLOW}$(echo "$result" | head -1)${NC}"
    fi
}

echo -e "${CYAN}═══════════════════════════════════════════════════════════${NC}"
echo -e "${CYAN}  🧬 EvoClaw End-to-End Flow Test${NC}"
echo -e "${CYAN}═══════════════════════════════════════════════════════════${NC}"
echo ""

# ─── Test 1: Orchestrator Health ─────────────────────────
echo -e "${YELLOW}📋 Test 1: Orchestrator Health${NC}"
RESULT=$(curl -s "$API/api/status" 2>&1)
check "API responds" "$RESULT" "version"
check "Has agents" "$RESULT" "agents"
check "Has models" "$RESULT" "models"
echo ""

# ─── Test 2: MQTT Broker Health ──────────────────────────
echo -e "${YELLOW}📋 Test 2: MQTT Broker Health${NC}"
RESULT=$(mosquitto_pub -h $BROKER -p $MQTT_PORT -t "evoclaw/test/ping" -m "pong" 2>&1 && echo "OK" || echo "FAIL")
check "MQTT broker accepts publish" "$RESULT" "OK"

RESULT=$(timeout 3 mosquitto_sub -h $BROKER -p $MQTT_PORT -t "evoclaw/test/reply" -C 1 &
  sleep 1
  mosquitto_pub -h $BROKER -p $MQTT_PORT -t "evoclaw/test/reply" -m "hello"
  wait 2>/dev/null)
check "MQTT pub/sub roundtrip" "$RESULT" "hello"
echo ""

# ─── Test 3: Agent Registration ──────────────────────────
echo -e "${YELLOW}📋 Test 3: Agent Registration${NC}"
RESULT=$(curl -s "$API/api/agents" 2>&1)
check "Agents list returns data" "$RESULT" "pi1-edge"
check "Agent has metrics" "$RESULT" "total_actions"

RESULT=$(curl -s -X POST "$API/api/agents/register" \
  -H "Content-Type: application/json" \
  -d '{"id":"e2e-test-agent","type":"monitor","host":"127.0.0.1"}' 2>&1)
check "Dynamic agent registration" "$RESULT" "registered"
echo ""

# ─── Test 4: Orchestrator → Broker → Agent (Command) ────
echo -e "${YELLOW}📋 Test 4: Command Flow (Orchestrator → Broker → Agent)${NC}"

# Listen for agent response in background
RESPONSE_FILE=$(mktemp)
timeout 10 mosquitto_sub -h $BROKER -p $MQTT_PORT -t "evoclaw/agents/$AGENT_ID/reports" -C 1 > "$RESPONSE_FILE" &
SUB_PID=$!
sleep 1

# Send command to agent via broker
mosquitto_pub -h $BROKER -p $MQTT_PORT -t "evoclaw/agents/$AGENT_ID/commands" \
  -m '{"command":"ping","payload":{},"request_id":"e2e-test-001"}'

# Wait for response
wait $SUB_PID 2>/dev/null || true
AGENT_RESPONSE=$(cat "$RESPONSE_FILE")
rm -f "$RESPONSE_FILE"

check "Agent received ping command" "$AGENT_RESPONSE" "pi1-edge"
check "Agent sent report back" "$AGENT_RESPONSE" "report_type"
echo ""

# ─── Test 5: Agent → Broker → Orchestrator (Report) ─────
echo -e "${YELLOW}📋 Test 5: Report Flow (Agent → Broker → Orchestrator)${NC}"

# Get current message count
BEFORE=$(curl -s "$API/api/agents" | python3 -c "import sys,json; d=json.load(sys.stdin); a=[x for x in d if x['id']=='$AGENT_ID']; print(a[0]['message_count'] if a else 0)" 2>/dev/null)

# Send a report as if from the agent
mosquitto_pub -h $BROKER -p $MQTT_PORT -t "evoclaw/agents/$AGENT_ID/reports" \
  -m "{\"agent_id\":\"$AGENT_ID\",\"agent_type\":\"monitor\",\"report_type\":\"result\",\"payload\":{\"test\":true},\"timestamp\":$(date +%s)}"
sleep 2

# Check message count increased
AFTER=$(curl -s "$API/api/agents" | python3 -c "import sys,json; d=json.load(sys.stdin); a=[x for x in d if x['id']=='$AGENT_ID']; print(a[0]['message_count'] if a else 0)" 2>/dev/null)

if [ "$AFTER" -gt "$BEFORE" ] 2>/dev/null; then
    PASS=$((PASS + 1)); TOTAL=$((TOTAL + 1))
    echo -e "  ${GREEN}✅ PASS${NC} — Orchestrator received and counted report ($BEFORE → $AFTER)"
else
    FAIL=$((FAIL + 1)); TOTAL=$((TOTAL + 1))
    echo -e "  ${RED}❌ FAIL${NC} — Report not counted ($BEFORE → $AFTER)"
fi
echo ""

# ─── Test 6: LLM Flow (Human → Orchestrator → Ollama) ───
echo -e "${YELLOW}📋 Test 6: LLM Flow (Human → Orchestrator → Ollama → Response)${NC}"

# Send a message that triggers LLM
mosquitto_pub -h $BROKER -p $MQTT_PORT -t "evoclaw/agents/$AGENT_ID/reports" \
  -m "{\"agent_id\":\"$AGENT_ID\",\"content\":\"Say hello world\",\"sent_at\":$(date +%s)}"
sleep 12

# Check orchestrator log for LLM response
RESULT=$(tail -30 /tmp/evoclaw-orchestrator.log 2>/dev/null | grep "agent responded" | tail -1)
check "LLM responded via Ollama" "$RESULT" "agent responded"
check "Response includes token count" "$RESULT" "tokens"
echo ""

# ─── Test 7: Full Round Trip (Dashboard verification) ──
echo -e "${YELLOW}📋 Test 7: Full Round Trip (Dashboard verification)${NC}"
RESULT=$(curl -s "$API/api/dashboard" 2>&1)
check "Dashboard endpoint returns data" "$RESULT" "total_messages"
check "Success rate tracked" "$RESULT" "success_rate"
check "Version present" "$RESULT" "0.1.0"
echo ""

# ─── Test 8: Agent Metrics Endpoint ─────────────────────
echo -e "${YELLOW}📋 Test 8: Agent Metrics${NC}"
RESULT=$(curl -s "$API/api/agents/$AGENT_ID/metrics" 2>&1)
check "Metrics endpoint responds" "$RESULT" "total_actions"
check "Agent uptime tracked" "$RESULT" "uptime"
echo ""

# ─── Test 9: Models Endpoint ────────────────────────────
echo -e "${YELLOW}📋 Test 9: Model Routing${NC}"
RESULT=$(curl -s "$API/api/models" 2>&1)
check "Models endpoint responds" "$RESULT" "qwen2.5"
echo ""

# ─── Test 10: Pi Agent Remote Health ────────────────────
echo -e "${YELLOW}📋 Test 10: Pi Agent Remote Health${NC}"
if command -v sshpass &>/dev/null; then
    RESULT=$(sshpass -p '123456' ssh -o StrictHostKeyChecking=no -o ConnectTimeout=5 admin@192.168.99.25 \
        'ps aux | grep evoclaw-agent | grep -v grep && free -m | grep Mem' 2>&1)
    check "Pi agent process running" "$RESULT" "evoclaw-agent"
    check "Pi has available memory" "$RESULT" "Mem:"
else
    echo -e "  ${YELLOW}⏭️ SKIP${NC} — sshpass not available"
    TOTAL=$((TOTAL + 2))
fi
echo ""

# ─── Results ────────────────────────────────────────────
echo -e "${CYAN}═══════════════════════════════════════════════════════════${NC}"
if [ $FAIL -eq 0 ]; then
    echo -e "  ${GREEN}🎉 ALL TESTS PASSED: $PASS/$TOTAL${NC}"
else
    echo -e "  ${YELLOW}Results: ${GREEN}$PASS passed${NC}, ${RED}$FAIL failed${NC}, $TOTAL total"
fi
echo -e "${CYAN}═══════════════════════════════════════════════════════════${NC}"
echo ""

# Exit with failure if any tests failed
[ $FAIL -eq 0 ] && exit 0 || exit 1
