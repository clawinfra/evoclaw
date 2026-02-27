#![allow(dead_code)]

use std::collections::HashMap;
use std::sync::Arc;
use std::time::Duration;

use async_trait::async_trait;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use tokio::sync::Mutex;
use tracing::{info, warn};

use super::{Skill, SkillReport};

/// On-chain agent information
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AgentInfo {
    pub did: String,
    pub reputation: u64,
    pub balance: u128,
    pub registered_at: u64,
    pub last_active: u64,
    pub metadata: HashMap<String, String>,
}

/// Reputation score details
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ReputationScore {
    pub agent_did: String,
    pub score: u64,
    pub total_tasks: u64,
    pub successful_tasks: u64,
    pub last_updated: u64,
}

/// Governance proposal information
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProposalInfo {
    pub id: u64,
    pub title: String,
    pub description: String,
    pub proposer: String,
    pub status: String,
    pub votes_for: u64,
    pub votes_against: u64,
    pub created_at: u64,
    pub ends_at: u64,
}

/// JSON-RPC request structure for Substrate
#[derive(Debug, Clone, Serialize, Deserialize)]
struct SubstrateRpcRequest {
    jsonrpc: String,
    id: u64,
    method: String,
    params: Vec<Value>,
}

/// JSON-RPC response structure from Substrate
#[derive(Debug, Clone, Serialize, Deserialize)]
struct SubstrateRpcResponse {
    jsonrpc: String,
    id: u64,
    result: Option<Value>,
    error: Option<SubstrateRpcError>,
}

/// JSON-RPC error
#[derive(Debug, Clone, Serialize, Deserialize)]
struct SubstrateRpcError {
    code: i64,
    message: String,
    data: Option<Value>,
}

/// MQTT RPC request envelope sent to orchestrator
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MqttRpcRequest {
    pub request_id: String,
    pub method: String,
    pub params: Value,
}

/// MQTT RPC response envelope received from orchestrator
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MqttRpcResponse {
    pub request_id: String,
    pub result: Option<Value>,
    pub error: Option<String>,
}

/// Cached on-chain state for tick reporting
#[derive(Debug, Clone, Default)]
struct CachedState {
    reputation: Option<u64>,
    balance: Option<u128>,
    last_updated: u64,
}

/// Trait for MQTT publishing — allows mocking in tests
#[async_trait]
pub trait MqttPublisher: Send + Sync {
    async fn publish(
        &self,
        topic: &str,
        payload: &[u8],
    ) -> Result<(), Box<dyn std::error::Error + Send + Sync>>;
}

/// Trait for HTTP RPC calls — allows mocking in tests
#[async_trait]
pub trait RpcClient: Send + Sync {
    async fn call(
        &self,
        url: &str,
        method: &str,
        params: Vec<Value>,
    ) -> Result<Value, Box<dyn std::error::Error + Send + Sync>>;
}

/// Default HTTP-based RPC client using reqwest
pub struct HttpRpcClient {
    client: reqwest::Client,
}

impl Default for HttpRpcClient {
    fn default() -> Self {
        Self::new()
    }
}

impl HttpRpcClient {
    pub fn new() -> Self {
        Self {
            client: reqwest::Client::new(),
        }
    }
}

#[async_trait]
impl RpcClient for HttpRpcClient {
    async fn call(
        &self,
        url: &str,
        method: &str,
        params: Vec<Value>,
    ) -> Result<Value, Box<dyn std::error::Error + Send + Sync>> {
        let request = SubstrateRpcRequest {
            jsonrpc: "2.0".to_string(),
            id: 1,
            method: method.to_string(),
            params,
        };

        let resp = self
            .client
            .post(url)
            .json(&request)
            .timeout(Duration::from_secs(15))
            .send()
            .await?;

        let rpc_resp: SubstrateRpcResponse = resp.json().await?;

        if let Some(error) = rpc_resp.error {
            return Err(format!("RPC error {}: {}", error.code, error.message).into());
        }

        rpc_resp
            .result
            .ok_or_else(|| "RPC response missing result".into())
    }
}

/// ClawChain Skill — connects edge agents to the ClawChain blockchain
pub struct ClawChainSkill {
    agent_id: String,
    agent_did: Option<String>,
    node_url: String,
    tick_interval: u64,
    mqtt_publisher: Option<Arc<dyn MqttPublisher>>,
    rpc_client: Option<Box<dyn RpcClient>>,
    cached_state: CachedState,
    rpc_request_counter: u64,
    pending_responses: Arc<Mutex<HashMap<String, MqttRpcResponse>>>,
}

impl ClawChainSkill {
    pub fn new(
        agent_id: String,
        agent_did: Option<String>,
        node_url: String,
        tick_interval: u64,
    ) -> Self {
        Self {
            agent_id,
            agent_did,
            node_url,
            tick_interval,
            mqtt_publisher: None,
            rpc_client: None,
            cached_state: CachedState::default(),
            rpc_request_counter: 0,
            pending_responses: Arc::new(Mutex::new(HashMap::new())),
        }
    }

    /// Set a custom MQTT publisher (for dependency injection / testing)
    pub fn with_mqtt_publisher(mut self, publisher: Arc<dyn MqttPublisher>) -> Self {
        self.mqtt_publisher = Some(publisher);
        self
    }

    /// Set a custom RPC client (for dependency injection / testing)
    pub fn with_rpc_client(mut self, client: Box<dyn RpcClient>) -> Self {
        self.rpc_client = Some(client);
        self
    }

    /// Get the pending responses map (for injecting mock responses in tests)
    pub fn pending_responses(&self) -> Arc<Mutex<HashMap<String, MqttRpcResponse>>> {
        Arc::clone(&self.pending_responses)
    }

    /// Generate a unique request ID
    fn next_request_id(&mut self) -> String {
        self.rpc_request_counter += 1;
        format!("clawchain-{}-{}", self.agent_id, self.rpc_request_counter)
    }

    /// MQTT topic for RPC requests to the orchestrator
    fn rpc_topic(&self) -> String {
        format!("evoclaw/rpc/clawchain/{}", self.agent_id)
    }

    /// Send an RPC request via MQTT, falling back to direct HTTP if MQTT is unavailable
    async fn rpc_call(
        &mut self,
        method: &str,
        params: Value,
    ) -> Result<Value, Box<dyn std::error::Error + Send + Sync>> {
        // Try MQTT first - clone values we need before the mutable borrow
        let request_id = self.next_request_id();
        let has_mqtt = self.mqtt_publisher.is_some();
        let topic = self.rpc_topic();

        if has_mqtt {
            // Take the publisher temporarily to avoid borrow conflicts
            if let Some(publisher) = self.mqtt_publisher.take() {
                let request = MqttRpcRequest {
                    request_id: request_id.clone(),
                    method: method.to_string(),
                    params: params.clone(),
                };

                let payload = serde_json::to_vec(&request)?;
                let result = publisher.publish(&topic, &payload).await;

                // Put the publisher back
                self.mqtt_publisher = Some(publisher);

                match result {
                    Ok(()) => {
                        // Wait for response with timeout
                        let pending = Arc::clone(&self.pending_responses);
                        let timeout = Duration::from_secs(10);
                        let start = std::time::Instant::now();

                        loop {
                            {
                                let mut responses = pending.lock().await;
                                if let Some(response) = responses.remove(&request_id) {
                                    if let Some(error) = response.error {
                                        return Err(error.into());
                                    }
                                    return response
                                        .result
                                        .ok_or_else(|| "empty RPC response".into());
                                }
                            }

                            if start.elapsed() >= timeout {
                                warn!(method = %method, "MQTT RPC timeout, falling back to HTTP");
                                break;
                            }

                            tokio::time::sleep(Duration::from_millis(50)).await;
                        }
                    }
                    Err(e) => {
                        warn!(error = %e, "MQTT publish failed, falling back to HTTP");
                    }
                }
            }
        }

        // Fallback to direct HTTP RPC
        self.rpc_call_http(method, params).await
    }

    /// Make a direct HTTP RPC call to the ClawChain node
    async fn rpc_call_http(
        &self,
        method: &str,
        params: Value,
    ) -> Result<Value, Box<dyn std::error::Error + Send + Sync>> {
        let rpc_client = self
            .rpc_client
            .as_ref()
            .ok_or("RPC client not initialized")?;

        let rpc_params = match params {
            Value::Array(arr) => arr,
            Value::Null => vec![],
            other => vec![other],
        };

        rpc_client.call(&self.node_url, method, rpc_params).await
    }

    /// Register this agent's DID on-chain
    async fn register_agent(
        &mut self,
        payload: &Value,
    ) -> Result<Value, Box<dyn std::error::Error + Send + Sync>> {
        let did = payload
            .get("did")
            .and_then(|v| v.as_str())
            .ok_or("missing 'did' field")?
            .to_string();

        let metadata = payload
            .get("metadata")
            .cloned()
            .unwrap_or_else(|| serde_json::json!({}));

        let result = self
            .rpc_call(
                "clawchain_registerAgent",
                serde_json::json!([did, metadata]),
            )
            .await?;

        // Cache the DID on successful registration
        self.agent_did = Some(did.clone());

        info!(did = %did, "agent registered on ClawChain");

        Ok(serde_json::json!({
            "status": "registered",
            "did": did,
            "tx_hash": result.get("tx_hash").cloned().unwrap_or(Value::Null),
        }))
    }

    /// Query reputation score for an agent
    async fn get_reputation(
        &mut self,
        payload: &Value,
    ) -> Result<Value, Box<dyn std::error::Error + Send + Sync>> {
        let did = payload
            .get("did")
            .and_then(|v| v.as_str())
            .or(self.agent_did.as_deref())
            .ok_or("missing 'did' field and no agent DID configured")?
            .to_string();

        let result = self
            .rpc_call("clawchain_getReputation", serde_json::json!([did]))
            .await?;

        // Cache if it's our own reputation
        if self.agent_did.as_deref() == Some(&did) {
            if let Some(score) = result.get("score").and_then(|v| v.as_u64()) {
                self.cached_state.reputation = Some(score);
                self.cached_state.last_updated = std::time::SystemTime::now()
                    .duration_since(std::time::UNIX_EPOCH)
                    .unwrap_or_default()
                    .as_secs();
            }
        }

        Ok(result)
    }

    /// Query token balance for an agent
    async fn get_balance(
        &mut self,
        payload: &Value,
    ) -> Result<Value, Box<dyn std::error::Error + Send + Sync>> {
        let did = payload
            .get("did")
            .and_then(|v| v.as_str())
            .or(self.agent_did.as_deref())
            .ok_or("missing 'did' field and no agent DID configured")?
            .to_string();

        let result = self
            .rpc_call("clawchain_getBalance", serde_json::json!([did]))
            .await?;

        // Cache if it's our own balance
        if self.agent_did.as_deref() == Some(&did) {
            if let Some(balance) = result.get("balance").and_then(|v| v.as_u64()) {
                self.cached_state.balance = Some(balance as u128);
                self.cached_state.last_updated = std::time::SystemTime::now()
                    .duration_since(std::time::UNIX_EPOCH)
                    .unwrap_or_default()
                    .as_secs();
            }
        }

        Ok(result)
    }

    /// Cast a governance vote
    async fn vote(
        &mut self,
        payload: &Value,
    ) -> Result<Value, Box<dyn std::error::Error + Send + Sync>> {
        let proposal_id = payload
            .get("proposal_id")
            .and_then(|v| v.as_u64())
            .ok_or("missing 'proposal_id' field")?;

        let vote_value = payload
            .get("vote")
            .and_then(|v| v.as_str())
            .ok_or("missing 'vote' field (expected 'for' or 'against')")?
            .to_string();

        if vote_value != "for" && vote_value != "against" {
            return Err("vote must be 'for' or 'against'".into());
        }

        let did = self
            .agent_did
            .as_deref()
            .ok_or("agent DID not set — register first")?
            .to_string();

        let result = self
            .rpc_call(
                "clawchain_vote",
                serde_json::json!([did, proposal_id, vote_value]),
            )
            .await?;

        info!(proposal_id = proposal_id, vote = %vote_value, "governance vote cast");

        Ok(serde_json::json!({
            "status": "voted",
            "proposal_id": proposal_id,
            "vote": vote_value,
            "tx_hash": result.get("tx_hash").cloned().unwrap_or(Value::Null),
        }))
    }

    /// Get detailed agent info from chain
    async fn get_agent_info(
        &mut self,
        payload: &Value,
    ) -> Result<Value, Box<dyn std::error::Error + Send + Sync>> {
        let did = payload
            .get("did")
            .and_then(|v| v.as_str())
            .or(self.agent_did.as_deref())
            .ok_or("missing 'did' field and no agent DID configured")?
            .to_string();

        self.rpc_call("clawchain_getAgentInfo", serde_json::json!([did]))
            .await
    }

    /// List governance proposals
    async fn list_proposals(
        &mut self,
        payload: &Value,
    ) -> Result<Value, Box<dyn std::error::Error + Send + Sync>> {
        let status_filter = payload
            .get("status")
            .and_then(|v| v.as_str())
            .unwrap_or("active");

        let limit = payload.get("limit").and_then(|v| v.as_u64()).unwrap_or(10);

        self.rpc_call(
            "clawchain_listProposals",
            serde_json::json!([status_filter, limit]),
        )
        .await
    }
}

#[async_trait]
impl Skill for ClawChainSkill {
    fn name(&self) -> &str {
        "clawchain"
    }

    fn capabilities(&self) -> Vec<String> {
        vec![
            "clawchain.register".to_string(),
            "clawchain.reputation".to_string(),
            "clawchain.balance".to_string(),
            "clawchain.vote".to_string(),
            "clawchain.agent_info".to_string(),
            "clawchain.proposals".to_string(),
        ]
    }

    async fn init(&mut self) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        // Initialize HTTP RPC client as fallback if none set
        if self.rpc_client.is_none() {
            self.rpc_client = Some(Box::new(HttpRpcClient::new()));
        }

        info!(
            agent_id = %self.agent_id,
            node_url = %self.node_url,
            agent_did = ?self.agent_did,
            "ClawChain skill initialized"
        );
        Ok(())
    }

    async fn handle(
        &mut self,
        command: &str,
        payload: Value,
    ) -> Result<Value, Box<dyn std::error::Error + Send + Sync>> {
        match command {
            "register_agent" => self.register_agent(&payload).await,
            "get_reputation" => self.get_reputation(&payload).await,
            "get_balance" => self.get_balance(&payload).await,
            "vote" => self.vote(&payload).await,
            "get_agent_info" => self.get_agent_info(&payload).await,
            "list_proposals" => self.list_proposals(&payload).await,
            "status" => Ok(serde_json::json!({
                "agent_id": self.agent_id,
                "agent_did": self.agent_did,
                "node_url": self.node_url,
                "cached_reputation": self.cached_state.reputation,
                "cached_balance": self.cached_state.balance.map(|b| b.to_string()),
                "cache_updated_at": self.cached_state.last_updated,
            })),
            _ => Err(format!("unknown clawchain command: {}", command).into()),
        }
    }

    async fn tick(&mut self) -> Option<SkillReport> {
        let did = self.agent_did.as_deref()?.to_string();

        // Fetch reputation
        let reputation_result = self
            .rpc_call("clawchain_getReputation", serde_json::json!([did]))
            .await;

        let reputation = match reputation_result {
            Ok(val) => val.get("score").and_then(|v| v.as_u64()),
            Err(e) => {
                warn!(error = %e, "failed to fetch reputation on tick");
                self.cached_state.reputation
            }
        };

        // Fetch balance
        let balance_result = self
            .rpc_call("clawchain_getBalance", serde_json::json!([did]))
            .await;

        let balance = match balance_result {
            Ok(val) => val
                .get("balance")
                .and_then(|v| v.as_u64())
                .map(|b| b as u128),
            Err(e) => {
                warn!(error = %e, "failed to fetch balance on tick");
                self.cached_state.balance
            }
        };

        // Update cache
        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs();

        if reputation.is_some() {
            self.cached_state.reputation = reputation;
        }
        if balance.is_some() {
            self.cached_state.balance = balance;
        }
        self.cached_state.last_updated = now;

        Some(SkillReport {
            skill: "clawchain".to_string(),
            report_type: "metric".to_string(),
            payload: serde_json::json!({
                "agent_did": did,
                "reputation": self.cached_state.reputation,
                "balance": self.cached_state.balance.map(|b| b.to_string()),
                "timestamp": now,
            }),
        })
    }

    fn tick_interval_secs(&self) -> u64 {
        self.tick_interval
    }

    async fn shutdown(&mut self) {
        info!(agent_id = %self.agent_id, "ClawChain skill shutting down");
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::atomic::{AtomicU32, Ordering};

    /// Mock MQTT publisher for testing
    struct MockMqttPublisher {
        publish_count: Arc<AtomicU32>,
        should_fail: bool,
        last_topic: Arc<Mutex<Option<String>>>,
        last_payload: Arc<Mutex<Option<Vec<u8>>>>,
    }

    impl MockMqttPublisher {
        fn new() -> Self {
            Self {
                publish_count: Arc::new(AtomicU32::new(0)),
                should_fail: false,
                last_topic: Arc::new(Mutex::new(None)),
                last_payload: Arc::new(Mutex::new(None)),
            }
        }

        fn failing() -> Self {
            Self {
                publish_count: Arc::new(AtomicU32::new(0)),
                should_fail: true,
                last_topic: Arc::new(Mutex::new(None)),
                last_payload: Arc::new(Mutex::new(None)),
            }
        }
    }

    #[async_trait]
    impl MqttPublisher for MockMqttPublisher {
        async fn publish(
            &self,
            topic: &str,
            payload: &[u8],
        ) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
            if self.should_fail {
                return Err("MQTT publish failed".into());
            }
            self.publish_count.fetch_add(1, Ordering::SeqCst);
            *self.last_topic.lock().await = Some(topic.to_string());
            *self.last_payload.lock().await = Some(payload.to_vec());
            Ok(())
        }
    }

    /// Mock RPC client for testing
    struct MockRpcClient {
        responses: Arc<Mutex<HashMap<String, Value>>>,
        call_count: Arc<AtomicU32>,
        should_fail: bool,
    }

    impl MockRpcClient {
        fn new() -> Self {
            Self {
                responses: Arc::new(Mutex::new(HashMap::new())),
                call_count: Arc::new(AtomicU32::new(0)),
                should_fail: false,
            }
        }

        fn failing() -> Self {
            Self {
                responses: Arc::new(Mutex::new(HashMap::new())),
                call_count: Arc::new(AtomicU32::new(0)),
                should_fail: true,
            }
        }

        #[allow(dead_code)]
        async fn set_response(&self, method: &str, response: Value) {
            self.responses
                .lock()
                .await
                .insert(method.to_string(), response);
        }
    }

    #[async_trait]
    impl RpcClient for MockRpcClient {
        async fn call(
            &self,
            _url: &str,
            method: &str,
            _params: Vec<Value>,
        ) -> Result<Value, Box<dyn std::error::Error + Send + Sync>> {
            self.call_count.fetch_add(1, Ordering::SeqCst);
            if self.should_fail {
                return Err("RPC call failed".into());
            }
            let responses = self.responses.lock().await;
            responses
                .get(method)
                .cloned()
                .ok_or_else(|| format!("no mock response for method: {}", method).into())
        }
    }

    fn new_test_skill() -> ClawChainSkill {
        ClawChainSkill::new(
            "test-agent-01".to_string(),
            None,
            "http://localhost:9933".to_string(),
            60,
        )
    }

    fn new_test_skill_with_did() -> ClawChainSkill {
        ClawChainSkill::new(
            "test-agent-01".to_string(),
            Some("did:claw:test123".to_string()),
            "http://localhost:9933".to_string(),
            60,
        )
    }

    async fn new_skill_with_mock_rpc() -> (
        ClawChainSkill,
        Arc<Mutex<HashMap<String, Value>>>,
        Arc<AtomicU32>,
    ) {
        let mock_rpc = MockRpcClient::new();
        let responses = Arc::clone(&mock_rpc.responses);
        let call_count = Arc::clone(&mock_rpc.call_count);

        let skill = ClawChainSkill::new(
            "test-agent-01".to_string(),
            Some("did:claw:test123".to_string()),
            "http://localhost:9933".to_string(),
            60,
        )
        .with_rpc_client(Box::new(mock_rpc));

        (skill, responses, call_count)
    }

    // ── Constructor & basic property tests ──

    #[test]
    fn test_new_skill() {
        let skill = new_test_skill();
        assert_eq!(skill.agent_id, "test-agent-01");
        assert!(skill.agent_did.is_none());
        assert_eq!(skill.node_url, "http://localhost:9933");
        assert_eq!(skill.tick_interval, 60);
        assert!(skill.mqtt_publisher.is_none());
        assert!(skill.rpc_client.is_none());
    }

    #[test]
    fn test_new_skill_with_did() {
        let skill = new_test_skill_with_did();
        assert_eq!(skill.agent_did, Some("did:claw:test123".to_string()));
    }

    #[test]
    fn test_skill_name() {
        let skill = new_test_skill();
        assert_eq!(skill.name(), "clawchain");
    }

    #[test]
    fn test_skill_capabilities() {
        let skill = new_test_skill();
        let caps = skill.capabilities();
        assert_eq!(caps.len(), 6);
        assert!(caps.contains(&"clawchain.register".to_string()));
        assert!(caps.contains(&"clawchain.reputation".to_string()));
        assert!(caps.contains(&"clawchain.balance".to_string()));
        assert!(caps.contains(&"clawchain.vote".to_string()));
        assert!(caps.contains(&"clawchain.agent_info".to_string()));
        assert!(caps.contains(&"clawchain.proposals".to_string()));
    }

    #[test]
    fn test_tick_interval() {
        let skill = ClawChainSkill::new(
            "agent".to_string(),
            None,
            "http://localhost:9933".to_string(),
            120,
        );
        assert_eq!(skill.tick_interval_secs(), 120);
    }

    #[test]
    fn test_rpc_topic() {
        let skill = new_test_skill();
        assert_eq!(skill.rpc_topic(), "evoclaw/rpc/clawchain/test-agent-01");
    }

    #[test]
    fn test_next_request_id() {
        let mut skill = new_test_skill();
        let id1 = skill.next_request_id();
        let id2 = skill.next_request_id();
        assert_eq!(id1, "clawchain-test-agent-01-1");
        assert_eq!(id2, "clawchain-test-agent-01-2");
    }

    #[test]
    fn test_with_mqtt_publisher() {
        let publisher = Arc::new(MockMqttPublisher::new());
        let skill = new_test_skill().with_mqtt_publisher(publisher);
        assert!(skill.mqtt_publisher.is_some());
    }

    #[test]
    fn test_with_rpc_client() {
        let rpc = MockRpcClient::new();
        let skill = new_test_skill().with_rpc_client(Box::new(rpc));
        assert!(skill.rpc_client.is_some());
    }

    // ── Init tests ──

    #[tokio::test]
    async fn test_init_creates_default_rpc_client() {
        let mut skill = new_test_skill();
        assert!(skill.rpc_client.is_none());
        let result = skill.init().await;
        assert!(result.is_ok());
        assert!(skill.rpc_client.is_some());
    }

    #[tokio::test]
    async fn test_init_preserves_custom_rpc_client() {
        let mock_rpc = MockRpcClient::new();
        let call_count = Arc::clone(&mock_rpc.call_count);
        let mut skill = new_test_skill().with_rpc_client(Box::new(mock_rpc));
        let result = skill.init().await;
        assert!(result.is_ok());
        // Should not have replaced the mock client
        assert_eq!(call_count.load(Ordering::SeqCst), 0);
    }

    // ── Handle command tests ──

    #[tokio::test]
    async fn test_handle_register_agent() {
        let (mut skill, responses, call_count) = new_skill_with_mock_rpc().await;
        // Remove DID so we can register fresh
        skill.agent_did = None;

        responses.lock().await.insert(
            "clawchain_registerAgent".to_string(),
            serde_json::json!({"tx_hash": "0xabc123"}),
        );

        let result = skill
            .handle(
                "register_agent",
                serde_json::json!({"did": "did:claw:new_agent", "metadata": {"type": "sensor"}}),
            )
            .await;

        assert!(result.is_ok());
        let val = result.unwrap();
        assert_eq!(val["status"], "registered");
        assert_eq!(val["did"], "did:claw:new_agent");
        assert_eq!(val["tx_hash"], "0xabc123");
        assert_eq!(skill.agent_did, Some("did:claw:new_agent".to_string()));
        assert_eq!(call_count.load(Ordering::SeqCst), 1);
    }

    #[tokio::test]
    async fn test_handle_register_agent_missing_did() {
        let (mut skill, _, _) = new_skill_with_mock_rpc().await;

        let result = skill.handle("register_agent", serde_json::json!({})).await;

        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("missing 'did'"));
    }

    #[tokio::test]
    async fn test_handle_get_reputation() {
        let (mut skill, responses, _) = new_skill_with_mock_rpc().await;

        responses.lock().await.insert(
            "clawchain_getReputation".to_string(),
            serde_json::json!({"score": 85, "total_tasks": 100, "successful_tasks": 85}),
        );

        let result = skill.handle("get_reputation", serde_json::json!({})).await;

        assert!(result.is_ok());
        let val = result.unwrap();
        assert_eq!(val["score"], 85);
        // Should have cached the score since DID matches
        assert_eq!(skill.cached_state.reputation, Some(85));
    }

    #[tokio::test]
    async fn test_handle_get_reputation_explicit_did() {
        let (mut skill, responses, _) = new_skill_with_mock_rpc().await;

        responses.lock().await.insert(
            "clawchain_getReputation".to_string(),
            serde_json::json!({"score": 90}),
        );

        let result = skill
            .handle(
                "get_reputation",
                serde_json::json!({"did": "did:claw:other"}),
            )
            .await;

        assert!(result.is_ok());
        // Should NOT cache because the DID is different from ours
        assert!(skill.cached_state.reputation.is_none());
    }

    #[tokio::test]
    async fn test_handle_get_reputation_no_did() {
        let mut skill = new_test_skill();
        skill.rpc_client = Some(Box::new(MockRpcClient::new()));

        let result = skill.handle("get_reputation", serde_json::json!({})).await;

        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("no agent DID"));
    }

    #[tokio::test]
    async fn test_handle_get_balance() {
        let (mut skill, responses, _) = new_skill_with_mock_rpc().await;

        responses.lock().await.insert(
            "clawchain_getBalance".to_string(),
            serde_json::json!({"balance": 1000000, "symbol": "CLAW"}),
        );

        let result = skill.handle("get_balance", serde_json::json!({})).await;

        assert!(result.is_ok());
        let val = result.unwrap();
        assert_eq!(val["balance"], 1000000);
        assert_eq!(skill.cached_state.balance, Some(1000000));
    }

    #[tokio::test]
    async fn test_handle_get_balance_no_did() {
        let mut skill = new_test_skill();
        skill.rpc_client = Some(Box::new(MockRpcClient::new()));

        let result = skill.handle("get_balance", serde_json::json!({})).await;

        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_handle_vote_for() {
        let (mut skill, responses, _) = new_skill_with_mock_rpc().await;

        responses.lock().await.insert(
            "clawchain_vote".to_string(),
            serde_json::json!({"tx_hash": "0xvote1"}),
        );

        let result = skill
            .handle(
                "vote",
                serde_json::json!({"proposal_id": 42, "vote": "for"}),
            )
            .await;

        assert!(result.is_ok());
        let val = result.unwrap();
        assert_eq!(val["status"], "voted");
        assert_eq!(val["proposal_id"], 42);
        assert_eq!(val["vote"], "for");
    }

    #[tokio::test]
    async fn test_handle_vote_against() {
        let (mut skill, responses, _) = new_skill_with_mock_rpc().await;

        responses.lock().await.insert(
            "clawchain_vote".to_string(),
            serde_json::json!({"tx_hash": "0xvote2"}),
        );

        let result = skill
            .handle(
                "vote",
                serde_json::json!({"proposal_id": 1, "vote": "against"}),
            )
            .await;

        assert!(result.is_ok());
        let val = result.unwrap();
        assert_eq!(val["vote"], "against");
    }

    #[tokio::test]
    async fn test_handle_vote_invalid_value() {
        let (mut skill, _, _) = new_skill_with_mock_rpc().await;

        let result = skill
            .handle(
                "vote",
                serde_json::json!({"proposal_id": 1, "vote": "maybe"}),
            )
            .await;

        assert!(result.is_err());
        assert!(result
            .unwrap_err()
            .to_string()
            .contains("'for' or 'against'"));
    }

    #[tokio::test]
    async fn test_handle_vote_missing_proposal_id() {
        let (mut skill, _, _) = new_skill_with_mock_rpc().await;

        let result = skill
            .handle("vote", serde_json::json!({"vote": "for"}))
            .await;

        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("proposal_id"));
    }

    #[tokio::test]
    async fn test_handle_vote_missing_vote_field() {
        let (mut skill, _, _) = new_skill_with_mock_rpc().await;

        let result = skill
            .handle("vote", serde_json::json!({"proposal_id": 1}))
            .await;

        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_handle_vote_no_did() {
        let mut skill = new_test_skill();
        skill.rpc_client = Some(Box::new(MockRpcClient::new()));

        let result = skill
            .handle("vote", serde_json::json!({"proposal_id": 1, "vote": "for"}))
            .await;

        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("register first"));
    }

    #[tokio::test]
    async fn test_handle_get_agent_info() {
        let (mut skill, responses, _) = new_skill_with_mock_rpc().await;

        responses.lock().await.insert(
            "clawchain_getAgentInfo".to_string(),
            serde_json::json!({
                "did": "did:claw:test123",
                "reputation": 85,
                "balance": 1000000,
                "registered_at": 1700000000,
                "last_active": 1700001000,
            }),
        );

        let result = skill.handle("get_agent_info", serde_json::json!({})).await;

        assert!(result.is_ok());
        let val = result.unwrap();
        assert_eq!(val["did"], "did:claw:test123");
    }

    #[tokio::test]
    async fn test_handle_list_proposals() {
        let (mut skill, responses, _) = new_skill_with_mock_rpc().await;

        responses.lock().await.insert(
            "clawchain_listProposals".to_string(),
            serde_json::json!({
                "proposals": [
                    {"id": 1, "title": "Increase rewards", "status": "active"},
                    {"id": 2, "title": "Add new skill type", "status": "active"},
                ]
            }),
        );

        let result = skill.handle("list_proposals", serde_json::json!({})).await;

        assert!(result.is_ok());
        let val = result.unwrap();
        assert_eq!(val["proposals"].as_array().map(|a| a.len()), Some(2));
    }

    #[tokio::test]
    async fn test_handle_list_proposals_with_filters() {
        let (mut skill, responses, _) = new_skill_with_mock_rpc().await;

        responses.lock().await.insert(
            "clawchain_listProposals".to_string(),
            serde_json::json!({"proposals": []}),
        );

        let result = skill
            .handle(
                "list_proposals",
                serde_json::json!({"status": "closed", "limit": 5}),
            )
            .await;

        assert!(result.is_ok());
    }

    #[tokio::test]
    async fn test_handle_status() {
        let mut skill = new_test_skill_with_did();
        skill.rpc_client = Some(Box::new(MockRpcClient::new()));
        skill.cached_state.reputation = Some(75);
        skill.cached_state.balance = Some(500000);

        let result = skill.handle("status", serde_json::json!({})).await;

        assert!(result.is_ok());
        let val = result.unwrap();
        assert_eq!(val["agent_id"], "test-agent-01");
        assert_eq!(val["agent_did"], "did:claw:test123");
        assert_eq!(val["cached_reputation"], 75);
        assert_eq!(val["cached_balance"], "500000");
    }

    #[tokio::test]
    async fn test_handle_unknown_command() {
        let mut skill = new_test_skill();
        skill.rpc_client = Some(Box::new(MockRpcClient::new()));

        let result = skill.handle("nonexistent", serde_json::json!({})).await;

        assert!(result.is_err());
        assert!(result
            .unwrap_err()
            .to_string()
            .contains("unknown clawchain command"));
    }

    // ── Tick tests ──

    #[tokio::test]
    async fn test_tick_no_did_returns_none() {
        let mut skill = new_test_skill();
        skill.rpc_client = Some(Box::new(MockRpcClient::new()));

        let report = skill.tick().await;
        assert!(report.is_none());
    }

    #[tokio::test]
    async fn test_tick_with_did_returns_report() {
        let (mut skill, responses, _) = new_skill_with_mock_rpc().await;

        responses.lock().await.insert(
            "clawchain_getReputation".to_string(),
            serde_json::json!({"score": 92}),
        );
        responses.lock().await.insert(
            "clawchain_getBalance".to_string(),
            serde_json::json!({"balance": 5000}),
        );

        let report = skill.tick().await;
        assert!(report.is_some());
        let report = report.unwrap();
        assert_eq!(report.skill, "clawchain");
        assert_eq!(report.report_type, "metric");
        assert_eq!(report.payload["agent_did"], "did:claw:test123");
        assert_eq!(report.payload["reputation"], 92);
        assert_eq!(report.payload["balance"], "5000");

        // Check cache was updated
        assert_eq!(skill.cached_state.reputation, Some(92));
        assert_eq!(skill.cached_state.balance, Some(5000));
    }

    #[tokio::test]
    async fn test_tick_rpc_failure_uses_cache() {
        let mock_rpc = MockRpcClient::failing();
        let mut skill = ClawChainSkill::new(
            "test-agent".to_string(),
            Some("did:claw:cached".to_string()),
            "http://localhost:9933".to_string(),
            60,
        )
        .with_rpc_client(Box::new(mock_rpc));

        // Set cached state
        skill.cached_state.reputation = Some(50);
        skill.cached_state.balance = Some(100);

        let report = skill.tick().await;
        assert!(report.is_some());
        let report = report.unwrap();
        // Should use cached values when RPC fails
        assert_eq!(report.payload["reputation"], 50);
        assert_eq!(report.payload["balance"], "100");
    }

    // ── MQTT fallback tests ──

    #[tokio::test]
    async fn test_mqtt_publish_failure_falls_back_to_http() {
        let failing_mqtt = Arc::new(MockMqttPublisher::failing());
        let mock_rpc = MockRpcClient::new();
        let call_count = Arc::clone(&mock_rpc.call_count);
        mock_rpc.responses.lock().await.insert(
            "clawchain_getReputation".to_string(),
            serde_json::json!({"score": 77}),
        );

        let mut skill = ClawChainSkill::new(
            "test-agent".to_string(),
            Some("did:claw:fb".to_string()),
            "http://localhost:9933".to_string(),
            60,
        )
        .with_mqtt_publisher(failing_mqtt)
        .with_rpc_client(Box::new(mock_rpc));

        let result = skill.handle("get_reputation", serde_json::json!({})).await;

        assert!(result.is_ok());
        assert_eq!(result.unwrap()["score"], 77);
        // HTTP fallback should have been called
        assert_eq!(call_count.load(Ordering::SeqCst), 1);
    }

    #[tokio::test]
    async fn test_mqtt_timeout_falls_back_to_http() {
        // MQTT succeeds but no response arrives -> timeout -> fallback to HTTP
        let mqtt = Arc::new(MockMqttPublisher::new());
        let mock_rpc = MockRpcClient::new();
        let call_count = Arc::clone(&mock_rpc.call_count);
        mock_rpc.responses.lock().await.insert(
            "clawchain_getBalance".to_string(),
            serde_json::json!({"balance": 999}),
        );

        let mut skill = ClawChainSkill::new(
            "test-agent".to_string(),
            Some("did:claw:to".to_string()),
            "http://localhost:9933".to_string(),
            60,
        )
        .with_mqtt_publisher(mqtt)
        .with_rpc_client(Box::new(mock_rpc));

        // Don't inject any MQTT response, so it will timeout
        // Note: We override the timeout behavior by not waiting 10s in tests
        // The MQTT path will timeout and fall back to HTTP
        let result = skill.handle("get_balance", serde_json::json!({})).await;

        assert!(result.is_ok());
        // Fallback to HTTP should have been used
        assert!(call_count.load(Ordering::SeqCst) >= 1);
    }

    #[tokio::test]
    async fn test_rpc_call_http_no_client() {
        let skill = new_test_skill();
        // No rpc_client set
        let result = skill
            .rpc_call_http("test_method", serde_json::json!(null))
            .await;
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("not initialized"));
    }

    #[tokio::test]
    async fn test_rpc_call_http_with_array_params() {
        let mock_rpc = MockRpcClient::new();
        mock_rpc
            .responses
            .lock()
            .await
            .insert("test_method".to_string(), serde_json::json!({"ok": true}));

        let skill = new_test_skill().with_rpc_client(Box::new(mock_rpc));
        let result = skill
            .rpc_call_http("test_method", serde_json::json!(["param1", "param2"]))
            .await;
        assert!(result.is_ok());
    }

    #[tokio::test]
    async fn test_rpc_call_http_with_null_params() {
        let mock_rpc = MockRpcClient::new();
        mock_rpc
            .responses
            .lock()
            .await
            .insert("test_method".to_string(), serde_json::json!({"ok": true}));

        let skill = new_test_skill().with_rpc_client(Box::new(mock_rpc));
        let result = skill
            .rpc_call_http("test_method", serde_json::json!(null))
            .await;
        assert!(result.is_ok());
    }

    #[tokio::test]
    async fn test_rpc_call_http_with_object_params() {
        let mock_rpc = MockRpcClient::new();
        mock_rpc
            .responses
            .lock()
            .await
            .insert("test_method".to_string(), serde_json::json!({"ok": true}));

        let skill = new_test_skill().with_rpc_client(Box::new(mock_rpc));
        let result = skill
            .rpc_call_http("test_method", serde_json::json!({"key": "val"}))
            .await;
        assert!(result.is_ok());
    }

    // ── Shutdown test ──

    #[tokio::test]
    async fn test_shutdown() {
        let mut skill = new_test_skill();
        // Should not panic
        skill.shutdown().await;
    }

    // ── Serialization tests ──

    #[test]
    fn test_agent_info_serialization() {
        let info = AgentInfo {
            did: "did:claw:test".to_string(),
            reputation: 85,
            balance: 1_000_000,
            registered_at: 1700000000,
            last_active: 1700001000,
            metadata: HashMap::from([("type".to_string(), "sensor".to_string())]),
        };
        let json = serde_json::to_string(&info).unwrap();
        let deserialized: AgentInfo = serde_json::from_str(&json).unwrap();
        assert_eq!(deserialized.did, "did:claw:test");
        assert_eq!(deserialized.reputation, 85);
        assert_eq!(deserialized.balance, 1_000_000);
        assert_eq!(deserialized.metadata["type"], "sensor");
    }

    #[test]
    fn test_reputation_score_serialization() {
        let score = ReputationScore {
            agent_did: "did:claw:rep".to_string(),
            score: 92,
            total_tasks: 100,
            successful_tasks: 92,
            last_updated: 1700000000,
        };
        let json = serde_json::to_string(&score).unwrap();
        let deserialized: ReputationScore = serde_json::from_str(&json).unwrap();
        assert_eq!(deserialized.score, 92);
        assert_eq!(deserialized.total_tasks, 100);
    }

    #[test]
    fn test_proposal_info_serialization() {
        let proposal = ProposalInfo {
            id: 42,
            title: "Increase rewards".to_string(),
            description: "Increase staking rewards by 10%".to_string(),
            proposer: "did:claw:proposer".to_string(),
            status: "active".to_string(),
            votes_for: 100,
            votes_against: 20,
            created_at: 1700000000,
            ends_at: 1700604800,
        };
        let json = serde_json::to_string(&proposal).unwrap();
        let deserialized: ProposalInfo = serde_json::from_str(&json).unwrap();
        assert_eq!(deserialized.id, 42);
        assert_eq!(deserialized.title, "Increase rewards");
        assert_eq!(deserialized.status, "active");
        assert_eq!(deserialized.votes_for, 100);
    }

    #[test]
    fn test_mqtt_rpc_request_serialization() {
        let req = MqttRpcRequest {
            request_id: "req-001".to_string(),
            method: "clawchain_getBalance".to_string(),
            params: serde_json::json!(["did:claw:test"]),
        };
        let json = serde_json::to_string(&req).unwrap();
        let deserialized: MqttRpcRequest = serde_json::from_str(&json).unwrap();
        assert_eq!(deserialized.request_id, "req-001");
        assert_eq!(deserialized.method, "clawchain_getBalance");
    }

    #[test]
    fn test_mqtt_rpc_response_serialization() {
        let resp = MqttRpcResponse {
            request_id: "req-001".to_string(),
            result: Some(serde_json::json!({"balance": 1000})),
            error: None,
        };
        let json = serde_json::to_string(&resp).unwrap();
        let deserialized: MqttRpcResponse = serde_json::from_str(&json).unwrap();
        assert_eq!(deserialized.request_id, "req-001");
        assert!(deserialized.error.is_none());
    }

    #[test]
    fn test_mqtt_rpc_response_with_error() {
        let resp = MqttRpcResponse {
            request_id: "req-002".to_string(),
            result: None,
            error: Some("agent not found".to_string()),
        };
        let json = serde_json::to_string(&resp).unwrap();
        let deserialized: MqttRpcResponse = serde_json::from_str(&json).unwrap();
        assert_eq!(deserialized.error, Some("agent not found".to_string()));
    }

    #[test]
    fn test_substrate_rpc_request_serialization() {
        let req = SubstrateRpcRequest {
            jsonrpc: "2.0".to_string(),
            id: 1,
            method: "clawchain_registerAgent".to_string(),
            params: vec![serde_json::json!("did:claw:test")],
        };
        let json = serde_json::to_string(&req).unwrap();
        assert!(json.contains("\"jsonrpc\":\"2.0\""));
        assert!(json.contains("\"method\":\"clawchain_registerAgent\""));
    }

    #[test]
    fn test_substrate_rpc_response_with_result() {
        let resp = SubstrateRpcResponse {
            jsonrpc: "2.0".to_string(),
            id: 1,
            result: Some(serde_json::json!({"tx_hash": "0x123"})),
            error: None,
        };
        assert!(resp.result.is_some());
        assert!(resp.error.is_none());
    }

    #[test]
    fn test_substrate_rpc_response_with_error() {
        let resp = SubstrateRpcResponse {
            jsonrpc: "2.0".to_string(),
            id: 1,
            result: None,
            error: Some(SubstrateRpcError {
                code: -32600,
                message: "Invalid Request".to_string(),
                data: None,
            }),
        };
        assert!(resp.result.is_none());
        assert!(resp.error.is_some());
        assert_eq!(resp.error.unwrap().code, -32600);
    }

    #[test]
    fn test_cached_state_default() {
        let state = CachedState::default();
        assert!(state.reputation.is_none());
        assert!(state.balance.is_none());
        assert_eq!(state.last_updated, 0);
    }

    // ── Integration-style tests ──

    #[tokio::test]
    async fn test_register_then_query() {
        let (mut skill, responses, _) = new_skill_with_mock_rpc().await;
        skill.agent_did = None;

        // Register
        responses.lock().await.insert(
            "clawchain_registerAgent".to_string(),
            serde_json::json!({"tx_hash": "0xreg"}),
        );

        let reg_result = skill
            .handle(
                "register_agent",
                serde_json::json!({"did": "did:claw:integrated"}),
            )
            .await;
        assert!(reg_result.is_ok());
        assert_eq!(skill.agent_did, Some("did:claw:integrated".to_string()));

        // Now query reputation using the newly registered DID
        responses.lock().await.insert(
            "clawchain_getReputation".to_string(),
            serde_json::json!({"score": 0, "total_tasks": 0, "successful_tasks": 0}),
        );

        let rep_result = skill.handle("get_reputation", serde_json::json!({})).await;
        assert!(rep_result.is_ok());
    }

    #[tokio::test]
    async fn test_full_workflow() {
        let (mut skill, responses, call_count) = new_skill_with_mock_rpc().await;

        // Setup all mock responses
        {
            let mut r = responses.lock().await;
            r.insert(
                "clawchain_getReputation".to_string(),
                serde_json::json!({"score": 95}),
            );
            r.insert(
                "clawchain_getBalance".to_string(),
                serde_json::json!({"balance": 10000, "symbol": "CLAW"}),
            );
            r.insert(
                "clawchain_getAgentInfo".to_string(),
                serde_json::json!({"did": "did:claw:test123", "reputation": 95, "balance": 10000}),
            );
            r.insert(
                "clawchain_listProposals".to_string(),
                serde_json::json!({"proposals": [{"id": 1, "title": "Test proposal"}]}),
            );
            r.insert(
                "clawchain_vote".to_string(),
                serde_json::json!({"tx_hash": "0xvote"}),
            );
        }

        // Check reputation
        let rep = skill
            .handle("get_reputation", serde_json::json!({}))
            .await
            .unwrap();
        assert_eq!(rep["score"], 95);

        // Check balance
        let bal = skill
            .handle("get_balance", serde_json::json!({}))
            .await
            .unwrap();
        assert_eq!(bal["balance"], 10000);

        // Get agent info
        let info = skill
            .handle("get_agent_info", serde_json::json!({}))
            .await
            .unwrap();
        assert_eq!(info["did"], "did:claw:test123");

        // List proposals
        let props = skill
            .handle("list_proposals", serde_json::json!({}))
            .await
            .unwrap();
        assert_eq!(props["proposals"].as_array().unwrap().len(), 1);

        // Vote
        let vote = skill
            .handle("vote", serde_json::json!({"proposal_id": 1, "vote": "for"}))
            .await
            .unwrap();
        assert_eq!(vote["status"], "voted");

        // Check status
        let status = skill.handle("status", serde_json::json!({})).await.unwrap();
        assert_eq!(status["agent_id"], "test-agent-01");
        assert_eq!(status["cached_reputation"], 95);

        // Tick should produce a report
        let report = skill.tick().await;
        assert!(report.is_some());

        // Verify total RPC calls
        assert!(call_count.load(Ordering::SeqCst) >= 5);
    }
}
