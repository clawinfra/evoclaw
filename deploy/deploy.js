#!/usr/bin/env node
/**
 * Deploy AgentRegistry to BSC Testnet
 * 
 * Usage: node deploy.js
 * 
 * Requires test BNB in the deployer wallet.
 * Get test BNB from: https://www.bnbchain.org/en/testnet-faucet
 */

const { ethers } = require('ethers');
const fs = require('fs');
const path = require('path');

const BSC_TESTNET_RPC = 'https://data-seed-prebsc-1-s1.binance.org:8545';
const CHAIN_ID = 97;

async function main() {
  // Load wallet
  const walletData = JSON.parse(fs.readFileSync(path.join(__dirname, 'wallet.json'), 'utf8'));
  
  // Load compiled contract
  const abi = JSON.parse(fs.readFileSync(
    path.join(__dirname, 'build/_home_bowen_evoclaw_contracts_AgentRegistry_sol_AgentRegistry.abi'), 'utf8'
  ));
  const bytecode = '0x' + fs.readFileSync(
    path.join(__dirname, 'build/_home_bowen_evoclaw_contracts_AgentRegistry_sol_AgentRegistry.bin'), 'utf8'
  ).trim();

  console.log('🔗 Connecting to BSC Testnet...');
  const provider = new ethers.JsonRpcProvider(BSC_TESTNET_RPC, CHAIN_ID);
  const wallet = new ethers.Wallet(walletData.privateKey, provider);
  
  console.log(`📍 Deployer: ${wallet.address}`);
  
  // Check balance
  const balance = await provider.getBalance(wallet.address);
  console.log(`💰 Balance: ${ethers.formatEther(balance)} tBNB`);
  
  if (balance === 0n) {
    console.error('\n❌ No test BNB! Get some from:');
    console.error('   https://www.bnbchain.org/en/testnet-faucet');
    console.error(`   Wallet: ${wallet.address}`);
    process.exit(1);
  }

  console.log('\n🚀 Deploying AgentRegistry...');
  const factory = new ethers.ContractFactory(abi, bytecode, wallet);
  
  const contract = await factory.deploy({
    gasLimit: 5000000,
  });
  
  console.log(`📝 Tx hash: ${contract.deploymentTransaction().hash}`);
  console.log('⏳ Waiting for confirmation...');
  
  await contract.waitForDeployment();
  const contractAddress = await contract.getAddress();
  
  console.log(`\n✅ AgentRegistry deployed!`);
  console.log(`📍 Contract: ${contractAddress}`);
  console.log(`🔍 Explorer: https://testnet.bscscan.com/address/${contractAddress}`);
  
  // Save deployment info
  const deployment = {
    network: 'bsc-testnet',
    chainId: CHAIN_ID,
    contractAddress,
    deployer: wallet.address,
    txHash: contract.deploymentTransaction().hash,
    deployedAt: new Date().toISOString(),
    abi: abi,
  };
  
  fs.writeFileSync(
    path.join(__dirname, 'deployment.json'),
    JSON.stringify(deployment, null, 2)
  );
  console.log('\n💾 Deployment info saved to deploy/deployment.json');

  // Register the first agent as a test
  console.log('\n🤖 Registering EvoClaw agent on-chain...');
  const registryContract = new ethers.Contract(contractAddress, abi, wallet);
  
  const tx = await registryContract.registerAgent(
    'evoclaw-orchestrator',
    'claude-sonnet-4',
    ['orchestration', 'evolution', 'trading', 'monitoring'],
    { gasLimit: 500000 }
  );
  
  console.log(`📝 Register tx: ${tx.hash}`);
  const receipt = await tx.wait();
  
  // Parse the AgentRegistered event
  const event = receipt.logs.find(log => {
    try {
      return registryContract.interface.parseLog(log)?.name === 'AgentRegistered';
    } catch { return false; }
  });
  
  if (event) {
    const parsed = registryContract.interface.parseLog(event);
    console.log(`✅ Agent registered! ID: ${parsed.args.agentId.slice(0, 18)}...`);
  }

  // Query agent count
  const count = await registryContract.getAgentCount();
  console.log(`📊 Total agents on-chain: ${count}`);
  
  console.log('\n🎉 Deployment complete!');
  console.log(`\nUpdate evoclaw.json with:`);
  console.log(`  "contractAddress": "${contractAddress}"`);
}

main().catch(err => {
  console.error('Deploy failed:', err.message);
  process.exit(1);
});
