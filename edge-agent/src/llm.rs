use reqwest::Client;
use serde::{Deserialize, Serialize};
use std::time::Duration;
use tracing::info;

/// LLM client for calling language models via Anthropic-compatible API
pub struct LLMClient {
    client: Client,
    base_url: String,
    api_key: String,
    model: String,
}

#[derive(Debug, Clone, Serialize)]
struct ChatRequest {
    model: String,
    max_tokens: u32,
    system: Option<String>,
    messages: Vec<ChatMessage>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct ChatMessage {
    role: String,
    content: String,
}

#[derive(Debug, Clone, Deserialize)]
struct ChatResponse {
    #[allow(dead_code)]
    id: Option<String>,
    content: Vec<ContentBlock>,
    model: Option<String>,
    #[allow(dead_code)]
    stop_reason: Option<String>,
    usage: Option<Usage>,
}

#[derive(Debug, Clone, Deserialize)]
struct ContentBlock {
    #[serde(rename = "type")]
    content_type: String,
    text: Option<String>,
}

#[derive(Debug, Clone, Deserialize)]
struct Usage {
    input_tokens: u32,
    output_tokens: u32,
}

#[derive(Debug, Clone, Deserialize)]
struct ErrorResponse {
    error: Option<ErrorDetail>,
    msg: Option<String>,
}

#[derive(Debug, Clone, Deserialize)]
struct ErrorDetail {
    #[serde(rename = "type")]
    #[allow(dead_code)]
    error_type: Option<String>,
    message: Option<String>,
}

/// Response from LLM completion
#[derive(Debug, Clone)]
pub struct LLMResponse {
    pub content: String,
    pub model: String,
    pub input_tokens: u32,
    pub output_tokens: u32,
}

impl LLMClient {
    /// Create a new LLM client
    pub fn new(base_url: &str, api_key: &str, model: &str) -> Self {
        Self {
            client: Client::builder()
                .timeout(Duration::from_secs(120))
                .build()
                .expect("failed to create HTTP client"),
            base_url: base_url.to_string(),
            api_key: api_key.to_string(),
            model: model.to_string(),
        }
    }

    /// Create client from environment variables
    pub fn from_env() -> Option<Self> {
        let base_url = std::env::var("LLM_BASE_URL").ok()?;
        let api_key = std::env::var("LLM_API_KEY").ok()?;
        let model = std::env::var("LLM_MODEL").unwrap_or_else(|_| "glm-4.7".to_string());
        
        Some(Self::new(&base_url, &api_key, &model))
    }

    /// Send a prompt to the LLM and get a response
    pub async fn complete(
        &self,
        prompt: &str,
        system_prompt: Option<&str>,
        max_tokens: u32,
    ) -> Result<LLMResponse, Box<dyn std::error::Error + Send + Sync>> {
        let url = format!("{}/v1/messages", self.base_url);

        let request = ChatRequest {
            model: self.model.clone(),
            max_tokens,
            system: system_prompt.map(|s| s.to_string()),
            messages: vec![ChatMessage {
                role: "user".to_string(),
                content: prompt.to_string(),
            }],
        };

        info!(
            model = %self.model,
            prompt_length = prompt.len(),
            "sending LLM request"
        );

        let response = self
            .client
            .post(&url)
            .header("Content-Type", "application/json")
            .header("x-api-key", &self.api_key)
            .header("anthropic-version", "2023-06-01")
            .json(&request)
            .send()
            .await?;

        let status = response.status();
        let body = response.text().await?;

        if !status.is_success() {
            // Try to parse error
            if let Ok(err) = serde_json::from_str::<ErrorResponse>(&body) {
                let msg = err.msg
                    .or_else(|| err.error.and_then(|e| e.message))
                    .unwrap_or_else(|| "unknown error".to_string());
                return Err(format!("LLM API error {}: {}", status, msg).into());
            }
            return Err(format!("LLM API error {}: {}", status, body).into());
        }

        let chat_response: ChatResponse = serde_json::from_str(&body)?;

        // Extract text content
        let content = chat_response
            .content
            .iter()
            .filter_map(|block| {
                if block.content_type == "text" {
                    block.text.clone()
                } else {
                    None
                }
            })
            .collect::<Vec<_>>()
            .join("");

        let usage = chat_response.usage.unwrap_or(Usage {
            input_tokens: 0,
            output_tokens: 0,
        });

        info!(
            model = %self.model,
            input_tokens = usage.input_tokens,
            output_tokens = usage.output_tokens,
            "LLM response received"
        );

        Ok(LLMResponse {
            content,
            model: chat_response.model.unwrap_or_else(|| self.model.clone()),
            input_tokens: usage.input_tokens,
            output_tokens: usage.output_tokens,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_llm_client_new() {
        let client = LLMClient::new(
            "https://api.example.com",
            "test-key",
            "test-model",
        );
        assert_eq!(client.base_url, "https://api.example.com");
        assert_eq!(client.api_key, "test-key");
        assert_eq!(client.model, "test-model");
    }

    #[test]
    fn test_chat_request_serialization() {
        let request = ChatRequest {
            model: "test".to_string(),
            max_tokens: 1000,
            system: Some("You are helpful".to_string()),
            messages: vec![ChatMessage {
                role: "user".to_string(),
                content: "Hello".to_string(),
            }],
        };

        let json = serde_json::to_string(&request).unwrap();
        assert!(json.contains("test"));
        assert!(json.contains("Hello"));
    }

    #[test]
    fn test_chat_response_deserialization() {
        let json = r#"{
            "id": "msg_123",
            "content": [{"type": "text", "text": "Hello!"}],
            "model": "glm-4.7",
            "stop_reason": "end_turn",
            "usage": {"input_tokens": 10, "output_tokens": 5}
        }"#;

        let response: ChatResponse = serde_json::from_str(json).unwrap();
        assert_eq!(response.content[0].text, Some("Hello!".to_string()));
        assert_eq!(response.model, Some("glm-4.7".to_string()));
    }
}
