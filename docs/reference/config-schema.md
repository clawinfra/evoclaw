# Complete Config Schema

Full JSON schema reference for `evoclaw.json`.

## Schema

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "EvoClaw Configuration",
  "type": "object",
  "properties": {
    "server": {
      "type": "object",
      "properties": {
        "port": {
          "type": "integer",
          "default": 8420,
          "description": "HTTP API and dashboard port"
        },
        "dataDir": {
          "type": "string",
          "default": "./data",
          "description": "Directory for persistent state"
        },
        "logLevel": {
          "type": "string",
          "enum": ["debug", "info", "warn", "error"],
          "default": "info",
          "description": "Logging verbosity"
        }
      }
    },
    "mqtt": {
      "type": "object",
      "properties": {
        "host": {
          "type": "string",
          "default": "0.0.0.0",
          "description": "MQTT broker host"
        },
        "port": {
          "type": "integer",
          "default": 1883,
          "description": "MQTT broker port"
        },
        "username": {
          "type": "string",
          "default": "",
          "description": "MQTT auth username"
        },
        "password": {
          "type": "string",
          "default": "",
          "description": "MQTT auth password"
        }
      }
    },
    "channels": {
      "type": "object",
      "properties": {
        "telegram": {
          "type": "object",
          "properties": {
            "enabled": { "type": "boolean", "default": false },
            "botToken": { "type": "string", "description": "Telegram Bot API token" }
          }
        },
        "whatsapp": {
          "type": "object",
          "properties": {
            "enabled": { "type": "boolean", "default": false },
            "allowFrom": {
              "type": "array",
              "items": { "type": "string" },
              "description": "Allowed phone numbers"
            }
          }
        }
      }
    },
    "models": {
      "type": "object",
      "properties": {
        "providers": {
          "type": "object",
          "additionalProperties": {
            "type": "object",
            "properties": {
              "baseUrl": { "type": "string" },
              "apiKey": { "type": "string" },
              "models": {
                "type": "array",
                "items": {
                  "type": "object",
                  "properties": {
                    "id": { "type": "string" },
                    "name": { "type": "string" },
                    "contextWindow": { "type": "integer" },
                    "costInput": { "type": "number", "description": "USD per million input tokens" },
                    "costOutput": { "type": "number", "description": "USD per million output tokens" },
                    "capabilities": {
                      "type": "array",
                      "items": {
                        "type": "string",
                        "enum": ["reasoning", "code", "vision"]
                      }
                    }
                  },
                  "required": ["id", "name"]
                }
              }
            }
          }
        },
        "routing": {
          "type": "object",
          "properties": {
            "simple": { "type": "string", "description": "Model for simple tasks (provider/model-id)" },
            "complex": { "type": "string", "description": "Model for complex tasks" },
            "critical": { "type": "string", "description": "Model for critical tasks" }
          }
        }
      }
    },
    "evolution": {
      "type": "object",
      "properties": {
        "enabled": { "type": "boolean", "default": true },
        "evalIntervalSec": { "type": "integer", "default": 3600, "description": "Evaluation interval in seconds" },
        "minSamplesForEval": { "type": "integer", "default": 10, "description": "Min actions before first eval" },
        "maxMutationRate": { "type": "number", "default": 0.2, "minimum": 0, "maximum": 1, "description": "Max parameter mutation rate" }
      }
    },
    "agents": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "id": { "type": "string", "description": "Unique agent identifier" },
          "name": { "type": "string", "description": "Display name" },
          "type": {
            "type": "string",
            "enum": ["orchestrator", "trader", "monitor", "governance"],
            "description": "Agent type"
          },
          "model": { "type": "string", "description": "Default model (provider/model-id)" },
          "systemPrompt": { "type": "string", "description": "LLM system prompt" },
          "skills": { "type": "array", "items": { "type": "string" } },
          "config": { "type": "object", "additionalProperties": { "type": "string" } },
          "container": {
            "type": "object",
            "properties": {
              "enabled": { "type": "boolean", "default": false },
              "image": { "type": "string" },
              "memoryMb": { "type": "integer", "default": 512 },
              "cpuShares": { "type": "integer", "default": 256 },
              "allowNet": { "type": "boolean", "default": true },
              "allowTools": { "type": "array", "items": { "type": "string" } },
              "mounts": {
                "type": "array",
                "items": {
                  "type": "object",
                  "properties": {
                    "hostPath": { "type": "string" },
                    "containerPath": { "type": "string" },
                    "readOnly": { "type": "boolean", "default": false }
                  }
                }
              }
            }
          }
        },
        "required": ["id", "type"]
      }
    }
  }
}
```

## Defaults

When no config file exists, EvoClaw creates this default:

```json
{
  "server": {
    "port": 8420,
    "dataDir": "./data",
    "logLevel": "info"
  },
  "mqtt": {
    "host": "0.0.0.0",
    "port": 1883
  },
  "evolution": {
    "enabled": true,
    "evalIntervalSec": 3600,
    "minSamplesForEval": 10,
    "maxMutationRate": 0.2
  },
  "models": {
    "routing": {
      "simple": "local/small",
      "complex": "anthropic/claude-sonnet",
      "critical": "anthropic/claude-opus"
    }
  }
}
```

## See Also

- [Configuration Guide](../getting-started/configuration.md)
- [Environment Variables](environment.md)
- [Genome Format](genome-format.md)
