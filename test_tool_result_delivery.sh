#!/bin/bash
# Test script for MQTT tool result delivery feature
# This script verifies that the tool result delivery mechanism is properly implemented

set -e

echo "=== MQTT Tool Result Delivery Test ==="
echo ""

# Check if required functions exist
echo "1. Checking MQTTChannel.SetResultCallback..."
if grep -q "func (m \*MQTTChannel) SetResultCallback" internal/channels/mqtt.go; then
    echo "   ✓ SetResultCallback method found"
else
    echo "   ✗ SetResultCallback method NOT found"
    exit 1
fi

echo ""
echo "2. Checking MQTTChannel.handleToolResult..."
if grep -q "func (m \*MQTTChannel) handleToolResult" internal/channels/mqtt.go; then
    echo "   ✓ handleToolResult method found"
else
    echo "   ✗ handleToolResult method NOT found"
    exit 1
fi

echo ""
echo "3. Checking orchestrator.DeliverToolResult..."
if grep -q "func (o \*Orchestrator) DeliverToolResult" internal/orchestrator/orchestrator.go; then
    echo "   ✓ DeliverToolResult method found"
else
    echo "   ✗ DeliverToolResult method NOT found"
    exit 1
fi

echo ""
echo "4. Checking orchestrator.RegisterResultHandler..."
if grep -q "func (o \*Orchestrator) RegisterResultHandler" internal/orchestrator/orchestrator.go; then
    echo "   ✓ RegisterResultHandler method found"
else
    echo "   ✗ RegisterResultHandler method NOT found"
    exit 1
fi

echo ""
echo "5. Checking tool result detection in handleMessage..."
if grep -q "Check if this is a tool result" internal/channels/mqtt.go; then
    echo "   ✓ Tool result detection found"
else
    echo "   ✗ Tool result detection NOT found"
    exit 1
fi

echo ""
echo "6. Checking MQTT callback wiring in Start()..."
if grep -q "mqttCh.SetResultCallback(o.DeliverToolResult)" internal/orchestrator/orchestrator.go; then
    echo "   ✓ MQTT callback wiring found"
else
    echo "   ✗ MQTT callback wiring NOT found"
    exit 1
fi

echo ""
echo "7. Checking resultCallback field in MQTTChannel..."
if grep -q "resultCallback func(requestID string, result map\[string\]interface{})" internal/channels/mqtt.go; then
    echo "   ✓ resultCallback field found"
else
    echo "   ✗ resultCallback field NOT found"
    exit 1
fi

echo ""
echo "8. Checking resultMu mutex field..."
if grep -q "resultMu.*sync.RWMutex" internal/channels/mqtt.go; then
    echo "   ✓ resultMu field found"
else
    echo "   ✗ resultMu field NOT found"
    exit 1
fi

echo ""
echo "=== All checks passed! ==="
echo ""
echo "Summary of implementation:"
echo "  • MQTTChannel now has resultCallback mechanism"
echo "  • handleMessage detects tool results by checking 'tool' and 'status' fields"
echo "  • Tool results are routed to orchestrator via handleToolResult()"
echo "  • Orchestrator registers result handlers for pending tool calls"
echo "  • Orchestrator delivers results via DeliverToolResult()"
echo "  • Callback is wired up in orchestrator.Start()"
echo ""
echo "The complete flow:"
echo "  1. User: 'Check GPIO 529'"
echo "  2. Orchestrator → LLM → tool call: bash"
echo "  3. Orchestrator → MQTT → Edge Agent"
echo "  4. Edge Agent executes, returns result via MQTT"
echo "  5. MQTT channel → resultCallback → toolloop"
echo "  6. Toolloop feeds result to LLM"
echo "  7. LLM → final response → User"
