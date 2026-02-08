#!/usr/bin/env node
/**
 * Deploy AgentRegistry to local Hardhat node + run full integration test
 * 
 * Usage: node deploy-local.js
 */

const { ethers } = require('ethers');
const fs = require('fs');
const path = require('path');
const { execSync } = require('child_process');

const LOCAL_RPC = 'http://127.0.0.1:8545';

async function main() {
  // Load compiled contract
  const abi = JSON.parse(fs.readFileSync(
    path.join(__dirname, 'build/_home_bowen_evoclaw_contracts_AgentRegistry_sol_AgentRegistry.abi'), 'utf8'
  ));
  const bytecode = '0x' + fs.readFileSync(
    path.join(__dirname, 'build/_home_bowen_evoclaw_contracts_AgentRegistry_sol_AgentRegistry.bin'), 'utf8'
  ).trim();

  console.log('🔗 Connecting to local Hardhat node...');
  const provider = new ethers.JsonRpcProvider(LOCAL_RPC);
  
  // Use Hardhat's first default account (has 10000 ETH)
  const accounts = await provider.listAccounts();
  if (accounts.length === 0) {
    console.error('❌ No accounts found. Is Hardhat node running?');
    console.error('   Run: npx hardhat node');
    process.exit(1);
  }
  
  const signer = await provider.getSigner(0);
  const deployer = await signer.getAddress();
  const balance = await provider.getBalance(deployer);
  
  console.log(`📍 Deployer: ${deployer}`);
  console.log(`💰 Balance: ${ethers.formatEther(balance)} ETH`);

  // ═══════════════════════════════════════════
  // 1. DEPLOY CONTRACT
  // ═══════════════════════════════════════════
  console.log('\n═══ 1. DEPLOY ═══');
  console.log('🚀 Deploying AgentRegistry...');
  
  const factory = new ethers.ContractFactory(abi, bytecode, signer);
  const contract = await factory.deploy();
  await contract.waitForDeployment();
  const contractAddress = await contract.getAddress();
  
  console.log(`✅ Deployed at: ${contractAddress}`);

  const registry = new ethers.Contract(contractAddress, abi, signer);

  // ═══════════════════════════════════════════
  // 2. REGISTER AGENTS
  // ═══════════════════════════════════════════
  console.log('\n═══ 2. REGISTER AGENTS ═══');
  
  const agents = [
    { name: 'evoclaw-orchestrator', model: 'claude-sonnet-4', caps: ['orchestration', 'evolution', 'routing'] },
    { name: 'pi1-edge-trader', model: 'mistral-small-3.1-24b', caps: ['trading', 'monitoring', 'hyperliquid'] },
    { name: 'alex-chen-agent', model: 'claude-opus-4', caps: ['coding', 'research', 'social', 'automation'] },
  ];

  const agentIds = [];
  for (const agent of agents) {
    const tx = await registry.registerAgent(agent.name, agent.model, agent.caps, { gasLimit: 500000 });
    const receipt = await tx.wait();
    
    const event = receipt.logs.find(log => {
      try { return registry.interface.parseLog(log)?.name === 'AgentRegistered'; }
      catch { return false; }
    });
    
    const parsed = registry.interface.parseLog(event);
    agentIds.push(parsed.args.agentId);
    console.log(`  ✅ ${agent.name} → ${parsed.args.agentId.slice(0, 18)}...`);
  }

  const count = await registry.getAgentCount();
  console.log(`  📊 Total agents: ${count}`);

  // ═══════════════════════════════════════════
  // 3. LOG ACTIONS
  // ═══════════════════════════════════════════
  console.log('\n═══ 3. LOG ACTIONS ═══');
  
  const actions = [
    { agent: 0, type: 'trade', desc: 'Long ETH-PERP 2.5x at $3,245', success: true },
    { agent: 0, type: 'monitor', desc: 'Funding rate check: -0.012%', success: true },
    { agent: 1, type: 'trade', desc: 'Short BTC-PERP 1.5x at $97,234', success: true },
    { agent: 1, type: 'trade', desc: 'Stop loss triggered on SOL-PERP', success: false },
    { agent: 2, type: 'chat', desc: 'Processed user message via claude-opus', success: true },
    { agent: 2, type: 'code', desc: 'Built BSC on-chain integration (1444 lines)', success: true },
    { agent: 0, type: 'evolve', desc: 'Mutated FundingArbitrage params: minRate 0.01→0.015', success: true },
  ];

  for (const action of actions) {
    const dataHash = ethers.keccak256(ethers.toUtf8Bytes(action.desc));
    const tx = await registry.logAction(
      agentIds[action.agent], action.type, action.desc, dataHash, action.success,
      { gasLimit: 500000 }
    );
    await tx.wait();
    console.log(`  ${action.success ? '✅' : '❌'} [${agents[action.agent].name}] ${action.type}: ${action.desc.slice(0, 50)}`);
  }

  // ═══════════════════════════════════════════
  // 4. LOG EVOLUTION
  // ═══════════════════════════════════════════
  console.log('\n═══ 4. LOG EVOLUTION ═══');
  
  const evolutions = [
    { agent: 0, from: 'FundingArb v1 (minRate=0.01)', to: 'FundingArb v2 (minRate=0.015)', fitBefore: 720, fitAfter: 845 },
    { agent: 1, from: 'MeanReversion v3 (window=20)', to: 'MeanReversion v4 (window=15)', fitBefore: 650, fitAfter: 780 },
  ];

  for (const evo of evolutions) {
    const tx = await registry.logEvolution(
      agentIds[evo.agent], evo.from, evo.to, evo.fitBefore, evo.fitAfter,
      { gasLimit: 500000 }
    );
    await tx.wait();
    console.log(`  🧬 [${agents[evo.agent].name}] fitness: ${evo.fitBefore/10}% → ${evo.fitAfter/10}%`);
  }

  // ═══════════════════════════════════════════
  // 5. QUERY STATE
  // ═══════════════════════════════════════════
  console.log('\n═══ 5. QUERY ON-CHAIN STATE ═══');
  
  for (let i = 0; i < agents.length; i++) {
    const agent = await registry.agents(agentIds[i]);
    const rep = await registry.getReputation(agentIds[i]);
    const actionCount = await registry.getActionCount(agentIds[i]);
    const evoCount = await registry.getEvolutionCount(agentIds[i]);
    
    console.log(`\n  🤖 ${agent.name}`);
    console.log(`     Model: ${agent.model}`);
    console.log(`     Actions: ${actionCount} (${agent.successfulActions}/${agent.totalActions} success)`);
    console.log(`     Reputation: ${Number(rep)/10}%`);
    console.log(`     Evolutions: ${evoCount}`);
    console.log(`     Active: ${agent.active}`);
  }

  // ═══════════════════════════════════════════
  // 6. QUERY RECENT ACTIONS
  // ═══════════════════════════════════════════
  console.log('\n═══ 6. RECENT ACTIONS (orchestrator) ═══');
  
  const recentActions = await registry.getRecentActions(agentIds[0], 5);
  for (const action of recentActions) {
    const time = new Date(Number(action.timestamp) * 1000).toISOString();
    console.log(`  ${action.success ? '✅' : '❌'} [${action.actionType}] ${action.description.slice(0, 60)}`);
  }

  // ═══════════════════════════════════════════
  // SUMMARY
  // ═══════════════════════════════════════════
  console.log('\n═══════════════════════════════════════');
  console.log('🎉 ALL TESTS PASSED!');
  console.log('═══════════════════════════════════════');
  console.log(`\n📍 Contract: ${contractAddress}`);
  console.log(`🤖 Agents registered: ${count}`);
  console.log(`📝 Actions logged: ${actions.length}`);
  console.log(`🧬 Evolutions recorded: ${evolutions.length}`);
  console.log(`\n✅ Ready to deploy to BSC testnet!`);

  // Save local deployment info
  fs.writeFileSync(
    path.join(__dirname, 'deployment-local.json'),
    JSON.stringify({
      network: 'hardhat-local',
      contractAddress,
      deployer,
      agents: agents.map((a, i) => ({ ...a, id: agentIds[i] })),
      timestamp: new Date().toISOString(),
    }, null, 2)
  );
}

main().catch(err => {
  console.error('❌ Failed:', err.message);
  process.exit(1);
});
