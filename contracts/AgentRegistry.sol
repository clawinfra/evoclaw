// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

/**
 * @title EvoClaw Agent Registry
 * @notice On-chain registry for autonomous AI agents. Agents register,
 *         log actions, evolve strategies, and build verifiable reputation.
 * @dev Deployed on BSC/opBNB for the Good Vibes Only hackathon.
 *
 * Key features:
 * - Agent registration with metadata (model, capabilities, owner)
 * - Immutable action log (every agent decision recorded on-chain)
 * - Evolution tracking (strategy mutations with fitness scores)
 * - Reputation scoring (derived from action success rate)
 * - Owner-gated agent management
 */
contract AgentRegistry {
    // ─── Types ───────────────────────────────────────

    struct Agent {
        bytes32 agentId;        // unique identifier (keccak256 of name)
        address owner;          // wallet that registered the agent
        string name;            // human-readable name
        string model;           // LLM model (e.g., "claude-sonnet-4")
        string[] capabilities;  // skills/capabilities
        uint256 registeredAt;
        uint256 totalActions;
        uint256 successfulActions;
        uint256 evolutionCount;
        bool active;
    }

    struct ActionLog {
        bytes32 agentId;
        string actionType;      // "trade", "monitor", "evolve", "chat"
        string description;     // human-readable summary
        bytes32 dataHash;       // hash of full action data (stored off-chain)
        bool success;
        uint256 timestamp;
    }

    struct Evolution {
        bytes32 agentId;
        uint256 generation;
        string fromStrategy;    // previous strategy hash/description
        string toStrategy;      // new strategy hash/description
        uint256 fitnessBefore;  // fitness score × 1000 (fixed point)
        uint256 fitnessAfter;
        uint256 timestamp;
    }

    // ─── State ───────────────────────────────────────

    mapping(bytes32 => Agent) public agents;
    bytes32[] public agentIds;

    ActionLog[] public actionLogs;
    Evolution[] public evolutions;

    mapping(bytes32 => uint256[]) public agentActionIndices;
    mapping(bytes32 => uint256[]) public agentEvolutionIndices;

    // ─── Events ──────────────────────────────────────

    event AgentRegistered(bytes32 indexed agentId, address indexed owner, string name, string model);
    event AgentDeactivated(bytes32 indexed agentId);
    event ActionLogged(bytes32 indexed agentId, string actionType, bool success, uint256 index);
    event AgentEvolved(bytes32 indexed agentId, uint256 generation, uint256 fitnessAfter);

    // ─── Modifiers ───────────────────────────────────

    modifier onlyAgentOwner(bytes32 agentId) {
        require(agents[agentId].owner == msg.sender, "Not agent owner");
        _;
    }

    modifier agentExists(bytes32 agentId) {
        require(agents[agentId].registeredAt > 0, "Agent not found");
        _;
    }

    // ─── Registration ────────────────────────────────

    /**
     * @notice Register a new AI agent on-chain
     * @param name Human-readable agent name
     * @param model LLM model identifier
     * @param capabilities List of agent capabilities
     */
    function registerAgent(
        string calldata name,
        string calldata model,
        string[] calldata capabilities
    ) external returns (bytes32 agentId) {
        agentId = keccak256(abi.encodePacked(name, msg.sender, block.timestamp));

        require(agents[agentId].registeredAt == 0, "Agent ID collision");

        agents[agentId] = Agent({
            agentId: agentId,
            owner: msg.sender,
            name: name,
            model: model,
            capabilities: capabilities,
            registeredAt: block.timestamp,
            totalActions: 0,
            successfulActions: 0,
            evolutionCount: 0,
            active: true
        });

        agentIds.push(agentId);

        emit AgentRegistered(agentId, msg.sender, name, model);
    }

    /**
     * @notice Deactivate an agent (owner only)
     */
    function deactivateAgent(bytes32 agentId)
        external
        onlyAgentOwner(agentId)
        agentExists(agentId)
    {
        agents[agentId].active = false;
        emit AgentDeactivated(agentId);
    }

    // ─── Action Logging ──────────────────────────────

    /**
     * @notice Log an agent action on-chain (immutable record)
     * @param agentId The agent performing the action
     * @param actionType Category (e.g., "trade", "monitor", "evolve")
     * @param description Human-readable summary
     * @param dataHash Hash of full action data (stored off-chain for gas efficiency)
     * @param success Whether the action succeeded
     */
    function logAction(
        bytes32 agentId,
        string calldata actionType,
        string calldata description,
        bytes32 dataHash,
        bool success
    )
        external
        onlyAgentOwner(agentId)
        agentExists(agentId)
    {
        require(agents[agentId].active, "Agent deactivated");

        uint256 index = actionLogs.length;

        actionLogs.push(ActionLog({
            agentId: agentId,
            actionType: actionType,
            description: description,
            dataHash: dataHash,
            success: success,
            timestamp: block.timestamp
        }));

        agentActionIndices[agentId].push(index);

        agents[agentId].totalActions++;
        if (success) {
            agents[agentId].successfulActions++;
        }

        emit ActionLogged(agentId, actionType, success, index);
    }

    // ─── Evolution Tracking ──────────────────────────

    /**
     * @notice Record an agent evolution event
     * @param agentId The agent that evolved
     * @param fromStrategy Description/hash of previous strategy
     * @param toStrategy Description/hash of new strategy
     * @param fitnessBefore Fitness score before (× 1000)
     * @param fitnessAfter Fitness score after (× 1000)
     */
    function logEvolution(
        bytes32 agentId,
        string calldata fromStrategy,
        string calldata toStrategy,
        uint256 fitnessBefore,
        uint256 fitnessAfter
    )
        external
        onlyAgentOwner(agentId)
        agentExists(agentId)
    {
        agents[agentId].evolutionCount++;

        uint256 index = evolutions.length;

        evolutions.push(Evolution({
            agentId: agentId,
            generation: agents[agentId].evolutionCount,
            fromStrategy: fromStrategy,
            toStrategy: toStrategy,
            fitnessBefore: fitnessBefore,
            fitnessAfter: fitnessAfter,
            timestamp: block.timestamp
        }));

        agentEvolutionIndices[agentId].push(index);

        emit AgentEvolved(agentId, agents[agentId].evolutionCount, fitnessAfter);
    }

    // ─── View Functions ──────────────────────────────

    /**
     * @notice Get agent reputation score (success rate × 1000)
     */
    function getReputation(bytes32 agentId) external view returns (uint256) {
        Agent storage agent = agents[agentId];
        if (agent.totalActions == 0) return 0;
        return (agent.successfulActions * 1000) / agent.totalActions;
    }

    /**
     * @notice Get total number of registered agents
     */
    function getAgentCount() external view returns (uint256) {
        return agentIds.length;
    }

    /**
     * @notice Get action count for an agent
     */
    function getActionCount(bytes32 agentId) external view returns (uint256) {
        return agentActionIndices[agentId].length;
    }

    /**
     * @notice Get evolution count for an agent
     */
    function getEvolutionCount(bytes32 agentId) external view returns (uint256) {
        return agentEvolutionIndices[agentId].length;
    }

    /**
     * @notice Get recent actions for an agent (last N)
     */
    function getRecentActions(bytes32 agentId, uint256 count)
        external
        view
        returns (ActionLog[] memory)
    {
        uint256[] storage indices = agentActionIndices[agentId];
        uint256 total = indices.length;
        uint256 start = total > count ? total - count : 0;
        uint256 resultLen = total - start;

        ActionLog[] memory result = new ActionLog[](resultLen);
        for (uint256 i = 0; i < resultLen; i++) {
            result[i] = actionLogs[indices[start + i]];
        }
        return result;
    }

    /**
     * @notice Get all agent IDs (for enumeration)
     */
    function getAllAgentIds() external view returns (bytes32[] memory) {
        return agentIds;
    }
}
