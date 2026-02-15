use serde::{Deserialize, Serialize};
use std::collections::HashMap;

/// Complete genome defining an agent's identity, skills, behavior, and constraints
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Genome {
    pub identity: GenomeIdentity,
    pub skills: HashMap<String, SkillGenome>,
    pub behavior: GenomeBehavior,
    pub constraints: GenomeConstraints,
    #[serde(default)]
    pub constraint_signature: Vec<u8>,
    #[serde(default)]
    pub owner_public_key: Vec<u8>,
}

/// Agent identity layer
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct GenomeIdentity {
    pub name: String,
    pub persona: String,
    pub voice: String, // concise, verbose, balanced, etc.
}

/// Per-skill genome with evolvable parameters
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SkillGenome {
    pub enabled: bool,
    #[serde(default)]
    pub weight: f64, // Layer 2: skill importance weight (0.0-1.0)
    #[serde(default)]
    pub strategies: Vec<String>,
    pub params: HashMap<String, serde_json::Value>,
    #[serde(default)]
    pub fitness: f64,
    #[serde(default)]
    pub version: u32,
    #[serde(default)]
    pub dependencies: Vec<String>, // Layer 2: skills this skill depends on
    #[serde(default)]
    pub eval_count: u32, // Layer 2: number of evaluations
    #[serde(default)]
    pub verified: bool, // VBR: last mutation verified
    #[serde(default)]
    pub vfm_score: f64, // VFM: value-for-money of last mutation
}

/// Behavioral traits
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct GenomeBehavior {
    pub risk_tolerance: f64, // 0.0-1.0
    pub verbosity: f64,      // 0.0-1.0
    pub autonomy: f64,       // 0.0-1.0
    #[serde(default)]
    pub prompt_style: String, // Layer 3: "concise", "detailed", "socratic"
    #[serde(default)]
    pub tool_preferences: HashMap<String, f64>, // Layer 3: tool usage weights
    #[serde(default)]
    pub response_patterns: Vec<String>, // Layer 3: evolved response templates
}

/// Hard constraints (non-evolvable)
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct GenomeConstraints {
    #[serde(default)]
    pub max_loss_usd: f64,
    #[serde(default)]
    pub allowed_assets: Vec<String>,
    #[serde(default)]
    pub blocked_actions: Vec<String>,
    #[serde(default)]
    pub max_divergence: f64, // ADL: max mutation distance from original
    #[serde(default)]
    pub min_vfm_score: f64, // VFM: minimum value-for-money threshold
}

impl Default for Genome {
    fn default() -> Self {
        Self {
            identity: GenomeIdentity {
                name: "unnamed-agent".to_string(),
                persona: "helpful, reliable".to_string(),
                voice: "balanced".to_string(),
            },
            skills: HashMap::new(),
            behavior: GenomeBehavior {
                risk_tolerance: 0.3,
                verbosity: 0.5,
                autonomy: 0.5,
                prompt_style: "balanced".to_string(),
                tool_preferences: HashMap::new(),
                response_patterns: vec![],
            },
            constraints: GenomeConstraints {
                max_loss_usd: 1000.0,
                allowed_assets: vec![],
                blocked_actions: vec![],
                max_divergence: 0.0,
                min_vfm_score: 0.0,
            },
            constraint_signature: vec![],
            owner_public_key: vec![],
        }
    }
}

#[allow(dead_code)]
impl Genome {
    /// Validate genome structure
    pub fn validate(&self) -> Result<(), String> {
        if self.behavior.risk_tolerance < 0.0 || self.behavior.risk_tolerance > 1.0 {
            return Err("risk_tolerance must be between 0 and 1".to_string());
        }
        if self.behavior.verbosity < 0.0 || self.behavior.verbosity > 1.0 {
            return Err("verbosity must be between 0 and 1".to_string());
        }
        if self.behavior.autonomy < 0.0 || self.behavior.autonomy > 1.0 {
            return Err("autonomy must be between 0 and 1".to_string());
        }
        if self.constraints.max_loss_usd < 0.0 {
            return Err("max_loss_usd cannot be negative".to_string());
        }
        Ok(())
    }

    /// Get an enabled skill by name
    pub fn get_skill(&self, skill_name: &str) -> Option<&SkillGenome> {
        self.skills.get(skill_name).filter(|s| s.enabled)
    }

    /// Get mutable reference to a skill
    pub fn get_skill_mut(&mut self, skill_name: &str) -> Option<&mut SkillGenome> {
        self.skills.get_mut(skill_name)
    }

    /// Set or update a skill
    pub fn set_skill(&mut self, skill_name: String, skill: SkillGenome) {
        self.skills.insert(skill_name, skill);
    }

    /// List all enabled skill names
    pub fn enabled_skills(&self) -> Vec<String> {
        self.skills
            .iter()
            .filter(|(_, skill)| skill.enabled)
            .map(|(name, _)| name.clone())
            .collect()
    }

    /// Update skill parameters from evolved genome
    pub fn update_skill_params(
        &mut self,
        skill_name: &str,
        params: HashMap<String, serde_json::Value>,
    ) {
        if let Some(skill) = self.skills.get_mut(skill_name) {
            skill.params = params;
            skill.version += 1;
        }
    }
}

#[allow(dead_code)]
impl SkillGenome {
    /// Get a parameter as a float64
    pub fn get_f64(&self, key: &str) -> Option<f64> {
        self.params.get(key).and_then(|v| v.as_f64())
    }

    /// Get a parameter as an integer
    pub fn get_i64(&self, key: &str) -> Option<i64> {
        self.params.get(key).and_then(|v| v.as_i64())
    }

    /// Get a parameter as a string
    pub fn get_string(&self, key: &str) -> Option<String> {
        self.params
            .get(key)
            .and_then(|v| v.as_str())
            .map(|s| s.to_string())
    }

    /// Get a parameter as a boolean
    pub fn get_bool(&self, key: &str) -> Option<bool> {
        self.params.get(key).and_then(|v| v.as_bool())
    }

    /// Get a parameter as a Vec<String>
    pub fn get_string_array(&self, key: &str) -> Option<Vec<String>> {
        self.params.get(key).and_then(|v| {
            v.as_array().map(|arr| {
                arr.iter()
                    .filter_map(|item| item.as_str().map(|s| s.to_string()))
                    .collect()
            })
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_default_genome() {
        let genome = Genome::default();
        assert_eq!(genome.identity.name, "unnamed-agent");
        assert!(genome.validate().is_ok());
    }

    #[test]
    fn test_genome_validation() {
        let mut genome = Genome::default();

        // Valid genome
        assert!(genome.validate().is_ok());

        // Invalid risk tolerance
        genome.behavior.risk_tolerance = 1.5;
        assert!(genome.validate().is_err());

        genome.behavior.risk_tolerance = 0.5;
        assert!(genome.validate().is_ok());

        // Negative max loss
        genome.constraints.max_loss_usd = -100.0;
        assert!(genome.validate().is_err());
    }

    #[test]
    fn test_skill_operations() {
        let mut genome = Genome::default();

        let mut params = HashMap::new();
        params.insert("threshold".to_string(), serde_json::json!(-0.1));
        params.insert("position_size".to_string(), serde_json::json!(100.0));

        let skill = SkillGenome {
            enabled: true,
            weight: 1.0,
            strategies: vec!["FundingArbitrage".to_string()],
            params,
            fitness: 0.75,
            version: 1,
            dependencies: vec![],
            eval_count: 0,
            verified: false,
            vfm_score: 0.0,
        };

        genome.set_skill("trading".to_string(), skill);

        // Test get_skill
        let retrieved = genome.get_skill("trading");
        assert!(retrieved.is_some());
        assert_eq!(retrieved.unwrap().fitness, 0.75);

        // Test enabled_skills
        let enabled = genome.enabled_skills();
        assert_eq!(enabled.len(), 1);
        assert_eq!(enabled[0], "trading");

        // Test disabled skill
        genome.skills.get_mut("trading").unwrap().enabled = false;
        assert!(genome.get_skill("trading").is_none());
    }

    #[test]
    fn test_skill_param_getters() {
        let mut params = HashMap::new();
        params.insert("threshold".to_string(), serde_json::json!(-0.1));
        params.insert("position_size".to_string(), serde_json::json!(100));
        params.insert("enabled".to_string(), serde_json::json!(true));
        params.insert("name".to_string(), serde_json::json!("test"));

        let skill = SkillGenome {
            enabled: true,
            weight: 1.0,
            strategies: vec![],
            params,
            fitness: 0.0,
            version: 1,
            dependencies: vec![],
            eval_count: 0,
            verified: false,
            vfm_score: 0.0,
        };

        assert_eq!(skill.get_f64("threshold"), Some(-0.1));
        assert_eq!(skill.get_i64("position_size"), Some(100));
        assert_eq!(skill.get_bool("enabled"), Some(true));
        assert_eq!(skill.get_string("name"), Some("test".to_string()));
    }

    #[test]
    fn test_json_serialization() {
        let mut genome = Genome::default();

        let mut params = HashMap::new();
        params.insert("threshold".to_string(), serde_json::json!(-0.1));

        let skill = SkillGenome {
            enabled: true,
            weight: 1.0,
            strategies: vec!["FundingArbitrage".to_string()],
            params,
            fitness: 0.75,
            version: 2,
            dependencies: vec![],
            eval_count: 0,
            verified: false,
            vfm_score: 0.0,
        };

        genome.set_skill("trading".to_string(), skill);

        // Serialize
        let json = serde_json::to_string(&genome).unwrap();

        // Deserialize
        let decoded: Genome = serde_json::from_str(&json).unwrap();

        assert_eq!(decoded.identity.name, genome.identity.name);
        let trading_skill = decoded.get_skill("trading").unwrap();
        assert_eq!(trading_skill.fitness, 0.75);
        assert_eq!(trading_skill.version, 2);
    }
}
