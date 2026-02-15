// Contract Configuration
const CONTRACT_ADDRESS = '0xD20522b083ea53E1B34fBed018bF1eCF8670EaCf';
const BSC_TESTNET_CHAIN_ID = '0x61'; // 97 in hex
const BSC_TESTNET_RPC = 'https://data-seed-prebsc-1-s1.binance.org:8545';

const CONTRACT_ABI = [
    "function registerAgent(string name, string model, string[] capabilities) returns (bytes32)",
    "function getAgentCount() view returns (uint256)",
    "function getAllAgentIds() view returns (bytes32[])",
    "function agents(bytes32) view returns (bytes32 agentId, address owner, string name, string model, uint256 registeredAt, uint256 totalActions, uint256 successfulActions, uint256 evolutionCount, bool active)",
    "function getReputation(bytes32 agentId) view returns (uint256)",
    "function logAction(bytes32 agentId, string actionType, string description, bytes32 dataHash, bool success)",
    "function getRecentActions(bytes32 agentId, uint256 count) view returns (tuple(bytes32 agentId, string actionType, string description, bytes32 dataHash, bool success, uint256 timestamp)[])",
    "event AgentRegistered(bytes32 indexed agentId, address indexed owner, string name, string model)"
];

// Global State
let provider;
let signer;
let contract;
let userAddress;

// Initialize
document.addEventListener('DOMContentLoaded', () => {
    initEventListeners();
    checkWalletConnection();
});

function initEventListeners() {
    // Wallet
    document.getElementById('connectBtn').addEventListener('click', connectWallet);
    document.getElementById('disconnectBtn').addEventListener('click', disconnectWallet);
    
    // Tabs
    document.querySelectorAll('.tab-btn').forEach(btn => {
        btn.addEventListener('click', () => switchTab(btn.dataset.tab));
    });
    
    // Forms
    document.getElementById('registerForm').addEventListener('submit', registerAgent);
    document.getElementById('refreshAgents').addEventListener('click', loadAgents);
    
    // Modal
    document.querySelector('.close').addEventListener('click', closeModal);
    document.getElementById('agentModal').addEventListener('click', (e) => {
        if (e.target.id === 'agentModal') closeModal();
    });
    
    // Check for account changes
    if (window.ethereum) {
        window.ethereum.on('accountsChanged', handleAccountsChanged);
        window.ethereum.on('chainChanged', () => window.location.reload());
    }
}

// Wallet Functions
async function checkWalletConnection() {
    if (typeof window.ethereum === 'undefined') {
        showStatus('registerStatus', 'MetaMask not detected. Please install MetaMask.', 'error');
        return;
    }
    
    try {
        const accounts = await window.ethereum.request({ method: 'eth_accounts' });
        if (accounts.length > 0) {
            await connectWallet();
        }
    } catch (error) {
        console.error('Error checking wallet:', error);
    }
}

async function connectWallet() {
    try {
        if (typeof window.ethereum === 'undefined') {
            alert('Please install MetaMask!');
            return;
        }

        // Request account access
        const accounts = await window.ethereum.request({ method: 'eth_requestAccounts' });
        userAddress = accounts[0];

        // Initialize ethers provider
        provider = new ethers.BrowserProvider(window.ethereum);
        signer = await provider.getSigner();
        
        // Check and switch to BSC Testnet
        const chainId = await window.ethereum.request({ method: 'eth_chainId' });
        if (chainId !== BSC_TESTNET_CHAIN_ID) {
            await switchToBSCTestnet();
        }
        
        // Initialize contract
        contract = new ethers.Contract(CONTRACT_ADDRESS, CONTRACT_ABI, signer);
        
        // Update UI
        await updateWalletUI();
        await loadStats();
        await loadAgents();
        
        document.getElementById('walletConnect').classList.add('hidden');
        document.getElementById('walletInfo').classList.remove('hidden');
    } catch (error) {
        console.error('Connection error:', error);
        showStatus('registerStatus', `Connection failed: ${error.message}`, 'error');
    }
}

async function switchToBSCTestnet() {
    try {
        await window.ethereum.request({
            method: 'wallet_switchEthereumChain',
            params: [{ chainId: BSC_TESTNET_CHAIN_ID }],
        });
    } catch (switchError) {
        // Chain not added, add it
        if (switchError.code === 4902) {
            try {
                await window.ethereum.request({
                    method: 'wallet_addEthereumChain',
                    params: [{
                        chainId: BSC_TESTNET_CHAIN_ID,
                        chainName: 'BSC Testnet',
                        nativeCurrency: {
                            name: 'tBNB',
                            symbol: 'tBNB',
                            decimals: 18
                        },
                        rpcUrls: [BSC_TESTNET_RPC],
                        blockExplorerUrls: ['https://testnet.bscscan.com/']
                    }]
                });
            } catch (addError) {
                throw new Error('Failed to add BSC Testnet');
            }
        } else {
            throw switchError;
        }
    }
}

async function updateWalletUI() {
    // Update address
    const shortAddress = `${userAddress.slice(0, 6)}...${userAddress.slice(-4)}`;
    document.getElementById('walletAddress').textContent = shortAddress;
    
    // Update balance
    const balance = await provider.getBalance(userAddress);
    const balanceBNB = ethers.formatEther(balance);
    document.getElementById('walletBalance').textContent = `${parseFloat(balanceBNB).toFixed(4)} tBNB`;
    
    // Update network
    document.getElementById('networkStatus').innerHTML = 
        '<span style="color: var(--accent-green);">✓ BSC Testnet</span>';
}

function disconnectWallet() {
    provider = null;
    signer = null;
    contract = null;
    userAddress = null;
    
    document.getElementById('walletConnect').classList.remove('hidden');
    document.getElementById('walletInfo').classList.add('hidden');
    document.getElementById('totalAgents').textContent = '-';
    document.getElementById('agentsList').innerHTML = '<div class="loading">Connect wallet to view agents</div>';
}

function handleAccountsChanged(accounts) {
    if (accounts.length === 0) {
        disconnectWallet();
    } else if (accounts[0] !== userAddress) {
        connectWallet();
    }
}

// Tab Functions
function switchTab(tabName) {
    // Update buttons
    document.querySelectorAll('.tab-btn').forEach(btn => {
        btn.classList.remove('active');
    });
    document.querySelector(`[data-tab="${tabName}"]`).classList.add('active');
    
    // Update content
    document.querySelectorAll('.tab-content').forEach(content => {
        content.classList.remove('active');
    });
    document.getElementById(`${tabName}Tab`).classList.add('active');
    
    // Load agents when switching to view tab
    if (tabName === 'view' && contract) {
        loadAgents();
    }
}

// Stats Functions
async function loadStats() {
    if (!contract) return;
    
    try {
        const count = await contract.getAgentCount();
        document.getElementById('totalAgents').textContent = count.toString();
    } catch (error) {
        console.error('Error loading stats:', error);
        document.getElementById('totalAgents').textContent = 'Error';
    }
}

// Agent Functions
async function registerAgent(e) {
    e.preventDefault();
    
    if (!contract) {
        showStatus('registerStatus', 'Please connect your wallet first', 'error');
        return;
    }
    
    const name = document.getElementById('agentName').value.trim();
    const model = document.getElementById('agentModel').value.trim();
    const capabilitiesInput = document.getElementById('agentCapabilities').value.trim();
    const capabilities = capabilitiesInput.split(',').map(c => c.trim()).filter(c => c);
    
    if (!name || !model || capabilities.length === 0) {
        showStatus('registerStatus', 'Please fill in all fields', 'error');
        return;
    }
    
    try {
        showStatus('registerStatus', 'Registering agent... Please confirm the transaction', 'info');
        
        const tx = await contract.registerAgent(name, model, capabilities);
        showStatus('registerStatus', 'Transaction submitted. Waiting for confirmation...', 'info');
        
        const receipt = await tx.wait();
        
        // Extract agent ID from event
        const event = receipt.logs.find(log => {
            try {
                const parsed = contract.interface.parseLog(log);
                return parsed.name === 'AgentRegistered';
            } catch {
                return false;
            }
        });
        
        let agentId = 'Unknown';
        if (event) {
            const parsed = contract.interface.parseLog(event);
            agentId = parsed.args.agentId;
        }
        
        showStatus('registerStatus', 
            `✓ Agent registered successfully!<br>Agent ID: ${agentId}<br>Transaction: ${receipt.hash}`, 
            'success');
        
        // Reset form
        document.getElementById('registerForm').reset();
        
        // Update stats
        await loadStats();
        
        // Switch to view tab after 2 seconds
        setTimeout(() => {
            switchTab('view');
        }, 2000);
        
    } catch (error) {
        console.error('Registration error:', error);
        let errorMsg = 'Registration failed';
        if (error.code === 'ACTION_REJECTED') {
            errorMsg = 'Transaction rejected by user';
        } else if (error.message) {
            errorMsg = error.message;
        }
        showStatus('registerStatus', errorMsg, 'error');
    }
}

async function loadAgents() {
    if (!contract) {
        document.getElementById('agentsList').innerHTML = 
            '<div class="loading">Connect wallet to view agents</div>';
        return;
    }
    
    try {
        document.getElementById('agentsList').innerHTML = '<div class="loading">Loading agents...</div>';
        
        const agentIds = await contract.getAllAgentIds();
        
        if (agentIds.length === 0) {
            document.getElementById('agentsList').innerHTML = 
                '<div class="loading">No agents registered yet. Be the first!</div>';
            return;
        }
        
        const agentsHTML = [];
        
        for (const agentId of agentIds) {
            try {
                const agent = await contract.agents(agentId);
                const reputation = await contract.getReputation(agentId);
                
                const registeredDate = new Date(Number(agent.registeredAt) * 1000).toLocaleDateString();
                const successRate = agent.totalActions > 0 
                    ? ((Number(agent.successfulActions) / Number(agent.totalActions)) * 100).toFixed(1)
                    : '0';
                
                agentsHTML.push(`
                    <div class="agent-card" onclick="showAgentDetails('${agentId}')">
                        <div class="agent-header">
                            <div class="agent-name">${agent.name}</div>
                            <div class="agent-status ${agent.active ? 'active' : 'inactive'}">
                                ${agent.active ? 'Active' : 'Inactive'}
                            </div>
                        </div>
                        <div class="agent-info">
                            <div class="agent-info-item">
                                <div class="agent-info-label">Model</div>
                                <div class="agent-info-value">${agent.model}</div>
                            </div>
                            <div class="agent-info-item">
                                <div class="agent-info-label">Reputation</div>
                                <div class="agent-info-value">${reputation.toString()}</div>
                            </div>
                            <div class="agent-info-item">
                                <div class="agent-info-label">Actions</div>
                                <div class="agent-info-value">${agent.totalActions.toString()}</div>
                            </div>
                            <div class="agent-info-item">
                                <div class="agent-info-label">Success Rate</div>
                                <div class="agent-info-value">${successRate}%</div>
                            </div>
                            <div class="agent-info-item">
                                <div class="agent-info-label">Evolutions</div>
                                <div class="agent-info-value">${agent.evolutionCount.toString()}</div>
                            </div>
                            <div class="agent-info-item">
                                <div class="agent-info-label">Registered</div>
                                <div class="agent-info-value">${registeredDate}</div>
                            </div>
                        </div>
                        <div class="agent-id">ID: ${agentId}</div>
                    </div>
                `);
            } catch (error) {
                console.error(`Error loading agent ${agentId}:`, error);
            }
        }
        
        document.getElementById('agentsList').innerHTML = agentsHTML.join('');
        
    } catch (error) {
        console.error('Error loading agents:', error);
        document.getElementById('agentsList').innerHTML = 
            '<div class="loading">Error loading agents. Please try again.</div>';
    }
}

async function showAgentDetails(agentId) {
    if (!contract) return;
    
    try {
        const agent = await contract.agents(agentId);
        const reputation = await contract.getReputation(agentId);
        const recentActions = await contract.getRecentActions(agentId, 5);
        
        const registeredDate = new Date(Number(agent.registeredAt) * 1000).toLocaleString();
        const successRate = agent.totalActions > 0 
            ? ((Number(agent.successfulActions) / Number(agent.totalActions)) * 100).toFixed(1)
            : '0';
        
        let actionsHTML = '<p style="color: var(--text-secondary);">No actions logged yet</p>';
        if (recentActions.length > 0) {
            actionsHTML = '<div style="margin-top: 1rem;">';
            recentActions.forEach(action => {
                const actionDate = new Date(Number(action.timestamp) * 1000).toLocaleString();
                const statusIcon = action.success ? '✓' : '✗';
                const statusColor = action.success ? 'var(--accent-green)' : '#ff4444';
                actionsHTML += `
                    <div style="padding: 0.75rem; background: rgba(255,255,255,0.03); border-radius: 8px; margin-bottom: 0.5rem;">
                        <div style="display: flex; justify-content: space-between; margin-bottom: 0.25rem;">
                            <strong>${action.actionType}</strong>
                            <span style="color: ${statusColor}">${statusIcon}</span>
                        </div>
                        <div style="color: var(--text-secondary); font-size: 0.875rem;">${action.description}</div>
                        <div style="color: var(--text-secondary); font-size: 0.75rem; margin-top: 0.25rem;">${actionDate}</div>
                    </div>
                `;
            });
            actionsHTML += '</div>';
        }
        
        const isOwner = agent.owner.toLowerCase() === userAddress.toLowerCase();
        const logActionButton = isOwner ? `
            <div style="margin-top: 1.5rem; padding-top: 1.5rem; border-top: 1px solid rgba(255,255,255,0.05);">
                <button class="btn btn-primary" onclick="logActionForAgent('${agentId}')">
                    <span class="btn-icon">📝</span>
                    Log Action
                </button>
            </div>
        ` : '';
        
        const html = `
            <h2 style="margin-bottom: 1.5rem;">${agent.name}</h2>
            <div style="display: grid; gap: 1rem;">
                <div>
                    <div class="label">Agent ID</div>
                    <div class="mono" style="word-break: break-all; font-size: 0.875rem;">${agentId}</div>
                </div>
                <div>
                    <div class="label">Owner</div>
                    <div class="mono" style="word-break: break-all; font-size: 0.875rem;">${agent.owner}</div>
                </div>
                <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 1rem;">
                    <div>
                        <div class="label">Model</div>
                        <div class="value">${agent.model}</div>
                    </div>
                    <div>
                        <div class="label">Status</div>
                        <div class="value" style="color: ${agent.active ? 'var(--accent-green)' : '#ff4444'}">
                            ${agent.active ? 'Active' : 'Inactive'}
                        </div>
                    </div>
                </div>
                <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 1rem;">
                    <div>
                        <div class="label">Reputation</div>
                        <div class="value">${reputation.toString()}</div>
                    </div>
                    <div>
                        <div class="label">Total Actions</div>
                        <div class="value">${agent.totalActions.toString()}</div>
                    </div>
                </div>
                <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 1rem;">
                    <div>
                        <div class="label">Successful Actions</div>
                        <div class="value">${agent.successfulActions.toString()}</div>
                    </div>
                    <div>
                        <div class="label">Success Rate</div>
                        <div class="value">${successRate}%</div>
                    </div>
                </div>
                <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 1rem;">
                    <div>
                        <div class="label">Evolutions</div>
                        <div class="value">${agent.evolutionCount.toString()}</div>
                    </div>
                    <div>
                        <div class="label">Registered</div>
                        <div class="value" style="font-size: 0.875rem;">${registeredDate}</div>
                    </div>
                </div>
                <div>
                    <div class="label" style="margin-bottom: 0.5rem;">Recent Actions</div>
                    ${actionsHTML}
                </div>
            </div>
            ${logActionButton}
        `;
        
        document.getElementById('agentDetails').innerHTML = html;
        document.getElementById('agentModal').classList.remove('hidden');
        
    } catch (error) {
        console.error('Error loading agent details:', error);
        alert('Error loading agent details');
    }
}

async function logActionForAgent(agentId) {
    const actionType = prompt('Enter action type (e.g., "task_execution", "learning", "optimization"):');
    if (!actionType) return;
    
    const description = prompt('Enter action description:');
    if (!description) return;
    
    const success = confirm('Was the action successful?');
    
    try {
        const dataHash = ethers.keccak256(ethers.toUtf8Bytes(description));
        
        const tx = await contract.logAction(agentId, actionType, description, dataHash, success);
        alert('Transaction submitted. Waiting for confirmation...');
        
        await tx.wait();
        alert('Action logged successfully!');
        
        closeModal();
        await loadAgents();
        
    } catch (error) {
        console.error('Error logging action:', error);
        alert(`Failed to log action: ${error.message}`);
    }
}

function closeModal() {
    document.getElementById('agentModal').classList.add('hidden');
}

// Utility Functions
function showStatus(elementId, message, type) {
    const statusEl = document.getElementById(elementId);
    statusEl.innerHTML = message;
    statusEl.className = `status-msg ${type}`;
    statusEl.classList.remove('hidden');
    
    if (type === 'success') {
        setTimeout(() => {
            statusEl.classList.add('hidden');
        }, 5000);
    }
}
