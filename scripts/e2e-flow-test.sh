#!/bin/bash
set -e
BROKER="localhost"; MQTT_PORT=1883; API="http://localhost:8420"; PASS=0; FAIL=0; TOTAL=0
GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'
check() { TOTAL=$((TOTAL+1)); if echo "$2" | grep -q "$3"; then PASS=$((PASS+1)); echo -e "  ${GREEN}✅${NC} $1"; else FAIL=$((FAIL+1)); echo -e "  ${RED}❌${NC} $1 (expected: $3)"; fi; }
AGENT_ID="${AGENT_ID:-}"
[ -z "$AGENT_ID" ] && command -v sshpass &>/dev/null && AGENT_ID=$(sshpass -p '123456' ssh -o StrictHostKeyChecking=no -o ConnectTimeout=3 admin@192.168.99.25 "cat ~/.evoclaw/agent.toml 2>/dev/null | grep agent_id | cut -d'\"' -f2" 2>/dev/null | head -1)
AGENT_ID="${AGENT_ID:-pi1-edge}"
echo -e "${CYAN}🧬 EvoClaw E2E Flow Test (agent: $AGENT_ID)${NC}\n"
echo -e "${YELLOW}1. Orchestrator${NC}"
R=$(curl -s "$API/api/status"); check "API responds" "$R" "version"; check "Has agents" "$R" "agents"; check "Models available" "$R" "models"
echo -e "${YELLOW}2. MQTT Broker${NC}"
R=$(mosquitto_pub -h $BROKER -p $MQTT_PORT -t "evoclaw/test" -m "ok" && echo "OK"); check "Publish works" "$R" "OK"
R=$(timeout 3 mosquitto_sub -h $BROKER -p $MQTT_PORT -t "evoclaw/test/rt" -C 1 & sleep 1; mosquitto_pub -h $BROKER -p $MQTT_PORT -t "evoclaw/test/rt" -m "roundtrip"; wait 2>/dev/null); check "Pub/sub roundtrip" "$R" "roundtrip"
echo -e "${YELLOW}3. Agent Registration${NC}"
R=$(curl -s "$API/api/agents"); check "Agents list" "$R" "total_actions"
R=$(curl -s -X POST "$API/api/agents/register" -H "Content-Type: application/json" -d '{"id":"e2e-test","type":"monitor","host":"127.0.0.1"}'); check "Dynamic registration" "$R" "registered"
echo -e "${YELLOW}4. Command → Broker → Agent → Response${NC}"
TMP=$(mktemp); timeout 10 mosquitto_sub -h $BROKER -p $MQTT_PORT -t "evoclaw/agents/$AGENT_ID/reports" -C 1 > "$TMP" & sleep 1
mosquitto_pub -h $BROKER -p $MQTT_PORT -t "evoclaw/agents/$AGENT_ID/commands" -m '{"command":"ping","payload":{},"request_id":"e2e-001"}'
wait 2>/dev/null; R=$(cat "$TMP"); rm -f "$TMP"
check "Agent ping->pong" "$R" "pong"; check "Report received" "$R" "report_type"
echo -e "${YELLOW}5. Agent Report → Orchestrator Metrics${NC}"
B=$(curl -s "$API/api/agents" | python3 -c "import sys,json;[print(a['message_count']) for a in json.load(sys.stdin) if a['id']=='pi1-edge']" 2>/dev/null)
mosquitto_pub -h $BROKER -p $MQTT_PORT -t "evoclaw/agents/pi1-edge/reports" -m "{\"agent_id\":\"pi1-edge\",\"agent_type\":\"monitor\",\"report_type\":\"result\",\"payload\":{},\"timestamp\":$(date +%s)}"; sleep 2
A=$(curl -s "$API/api/agents" | python3 -c "import sys,json;[print(a['message_count']) for a in json.load(sys.stdin) if a['id']=='pi1-edge']" 2>/dev/null)
[ "${A:-0}" -gt "${B:-0}" ] 2>/dev/null && { PASS=$((PASS+1)); TOTAL=$((TOTAL+1)); echo -e "  ${GREEN}✅${NC} Metrics updated ($B->$A)"; } || { FAIL=$((FAIL+1)); TOTAL=$((TOTAL+1)); echo -e "  ${RED}❌${NC} Metrics not updated ($B->$A)"; }
echo -e "${YELLOW}6. LLM Flow (Human → Ollama)${NC}"
mosquitto_pub -h $BROKER -p $MQTT_PORT -t "evoclaw/agents/pi1-edge/reports" -m "{\"agent_id\":\"pi1-edge\",\"content\":\"Say hi\",\"sent_at\":$(date +%s)}"; sleep 12
R=$(tail -30 /tmp/evoclaw-orchestrator.log | grep "agent responded" | tail -1)
check "Ollama responded" "$R" "agent responded"; check "Token count" "$R" "tokens"
echo -e "${YELLOW}7. Dashboard API${NC}"
R=$(curl -s "$API/api/dashboard"); check "Dashboard data" "$R" "total_messages"; check "Success rate" "$R" "success_rate"
echo -e "${YELLOW}8. Agent Metrics API${NC}"
R=$(curl -s "$API/api/agents/pi1-edge/metrics"); check "Metrics endpoint" "$R" "total_actions"; check "Uptime tracked" "$R" "uptime"
echo -e "${YELLOW}9. Models${NC}"
R=$(curl -s "$API/api/models"); check "Models listed" "$R" "qwen2.5"
echo -e "${YELLOW}10. Pi Remote Health${NC}"
if command -v sshpass &>/dev/null; then
  R=$(sshpass -p '123456' ssh -o StrictHostKeyChecking=no -o ConnectTimeout=5 admin@192.168.99.25 'ps aux | grep evoclaw-agent | grep -v grep && free -m | grep Mem' 2>&1)
  check "Pi agent running" "$R" "evoclaw-agent"; check "Pi memory OK" "$R" "Mem:"
else echo "  SKIP (no sshpass)"; TOTAL=$((TOTAL+2)); fi
echo -e "\n${CYAN}Results:${NC}"
[ $FAIL -eq 0 ] && echo -e "  ${GREEN}ALL PASSED: $PASS/$TOTAL${NC}" || echo -e "  ${GREEN}$PASS passed${NC} / ${RED}$FAIL failed${NC} / $TOTAL total"
[ $FAIL -eq 0 ]
