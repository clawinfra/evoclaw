# Build a Companion Device Agent

This guide covers creating a companion agent for devices like smart toys, home hubs, or elderly care devices.

## The Vision

> Put EvoClaw in a teddy bear → it becomes a companion.

A companion agent:
- Has a personality that evolves based on user interactions
- Remembers conversations and preferences
- Runs on minimal hardware (Raspberry Pi Zero: $5–$15)
- Communicates via voice (with external STT/TTS)

## Hardware Reference Design

### Basic Companion (~$42 BOM)

| Component | Part | Price |
|-----------|------|-------|
| Computer | Raspberry Pi Zero 2W | $15 |
| Microphone | USB MEMS mic | $8 |
| Speaker | 3W mini speaker + amp | $7 |
| Power | USB-C power supply | $7 |
| Enclosure | 3D printed case | $5 |

### Home Hub (~$85 BOM)

| Component | Part | Price |
|-----------|------|-------|
| Computer | Raspberry Pi 4 (2GB) | $35 |
| Microphone | ReSpeaker 2-mic array | $12 |
| Speaker | 5W stereo speakers | $15 |
| Display | 3.5" LCD (optional) | $15 |
| Power | USB-C power supply | $8 |

## Step 1: Set Up the Orchestrator

Define a companion agent:

```json
{
  "agents": [
    {
      "id": "teddy-companion",
      "name": "Teddy",
      "type": "orchestrator",
      "model": "anthropic/claude-sonnet-4-20250514",
      "systemPrompt": "You are Teddy, a warm and friendly companion. You speak in a cheerful, encouraging way. You remember details about the person you're talking to and bring them up naturally. You tell stories, play word games, and always listen with empathy. Keep responses concise (2-3 sentences) for voice output.",
      "skills": ["chat", "storytelling", "games"],
      "config": {
        "max_tokens": "150",
        "voice_enabled": "true"
      }
    }
  ]
}
```

## Step 2: Configure the Edge Agent

```toml
agent_id = "teddy-companion"
agent_type = "monitor"  # Uses monitor type for simple event loop

[mqtt]
broker = "192.168.1.100"  # Orchestrator IP
port = 1883
keep_alive_secs = 30

[orchestrator]
url = "http://192.168.1.100:8420"
```

## Step 3: Voice Pipeline

The companion needs a voice pipeline. This runs alongside the edge agent:

```
Microphone → STT (cloud) → Text → MQTT → Orchestrator → LLM → MQTT → TTS (cloud) → Speaker
```

### Speech-to-Text Options

| Service | Latency | Cost | Quality |
|---------|---------|------|---------|
| Whisper API (OpenAI) | ~1s | $0.006/min | Excellent |
| Deepgram | ~300ms | $0.0043/min | Excellent |
| Google Cloud STT | ~500ms | $0.006/min | Good |
| Whisper.cpp (local) | ~2-5s on Pi 4 | Free | Good |

### Text-to-Speech Options

| Service | Latency | Cost | Quality |
|---------|---------|------|---------|
| ElevenLabs | ~1s | $0.30/1K chars | Excellent |
| OpenAI TTS | ~500ms | $15/1M chars | Very good |
| Piper (local) | ~200ms on Pi 4 | Free | Good |

### Example Voice Script (Python)

```python
#!/usr/bin/env python3
"""Simple voice pipeline for EvoClaw companion."""

import sounddevice as sd
import numpy as np
import requests
import paho.mqtt.client as mqtt
import json

AGENT_ID = "teddy-companion"
ORCHESTRATOR = "http://localhost:8420"
MQTT_BROKER = "localhost"

def record_audio(duration=5, samplerate=16000):
    """Record audio from microphone."""
    audio = sd.rec(int(duration * samplerate), samplerate=samplerate,
                   channels=1, dtype='int16')
    sd.wait()
    return audio

def transcribe(audio_data):
    """Send audio to Whisper API for transcription."""
    # Save to temp file, send to API
    # Returns transcribed text
    pass

def speak(text):
    """Convert text to speech and play."""
    # Send to TTS API, get audio back, play through speaker
    pass

def on_message(client, userdata, msg):
    """Handle MQTT response from orchestrator."""
    data = json.loads(msg.payload)
    if data.get("type") == "response":
        speak(data["content"])

# Main loop
client = mqtt.Client()
client.connect(MQTT_BROKER, 1883)
client.subscribe(f"evoclaw/agents/{AGENT_ID}/reports")
client.on_message = on_message
client.loop_start()

while True:
    audio = record_audio()
    text = transcribe(audio)
    if text:
        # Send to orchestrator via MQTT
        client.publish(
            f"evoclaw/agents/{AGENT_ID}/commands",
            json.dumps({"type": "message", "content": text})
        )
```

## Step 4: Personality Evolution

The companion agent evolves its personality over time:

- **Remembers preferences** — "You mentioned you like dinosaurs last time!"
- **Adjusts tone** — More energetic for playful interactions, calmer for bedtime
- **Learns timing** — Knows when the user is typically active
- **Adapts vocabulary** — Matches the user's language level

The evolution engine tracks:
- Conversation length (engagement)
- User sentiment (positive/negative responses)
- Topic preferences (what the user talks about most)

## Step 5: Deploy to Device

```bash
# Cross-compile for Pi Zero (ARMv7)
cd edge-agent
rustup target add armv7-unknown-linux-gnueabihf
cargo build --release --target armv7-unknown-linux-gnueabihf

# Copy to Pi
scp target/armv7-unknown-linux-gnueabihf/release/evoclaw-agent pi@teddy-pi:~/
scp agent.toml pi@teddy-pi:~/
scp voice_pipeline.py pi@teddy-pi:~/

# Set up as systemd service
ssh pi@teddy-pi
sudo cat > /etc/systemd/system/evoclaw-companion.service << EOF
[Unit]
Description=EvoClaw Companion Agent
After=network.target

[Service]
ExecStart=/home/pi/evoclaw-agent --config /home/pi/agent.toml
Restart=always
User=pi

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl enable evoclaw-companion
sudo systemctl start evoclaw-companion
```

## Safety Boundaries

For companion agents, especially those interacting with children or elderly:

- **Content filtering** — Block inappropriate content in system prompt
- **Time limits** — Configurable session duration limits
- **Volume limits** — Keep audio output at safe levels
- **Emergency contacts** — Notify caregivers if distress detected
- **Privacy** — Conversations are stored locally, never shared

## See Also

- [Architecture Overview](../architecture/overview.md)
- [Edge Agent](../architecture/edge-agent.md)
- [Deployment Guide](deployment.md)
- [Philosophy](https://github.com/clawinfra/evoclaw/blob/main/docs/PHILOSOPHY.md)
