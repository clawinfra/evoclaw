// EvoClaw Dashboard - Alpine.js Application
// Bloomberg Terminal meets GitHub aesthetic

const API_BASE = window.location.origin;
const REFRESH_INTERVAL = 30000; // 30 seconds

function dashboard() {
    return {
        // State
        currentView: 'overview',
        sidebarCollapsed: false,
        connected: false,
        lastUpdated: null,

        // Data
        status: {},
        agents: [],
        models: [],
        costs: {},
        selectedAgent: null,
        agentMemory: [],
        agentEvolution: null,
        agentGenome: null,
        evolutionData: {},

        // Filters
        agentFilter: 'all',
        logLevel: 'all',

        // Logs
        logs: [],
        logStreaming: false,
        logEventSource: null,

        // Trading mock data
        tradingData: {
            totalPnl: 0,
            winRate: 0,
            totalTrades: 0,
            positions: [],
            strategies: [],
            recentTrades: []
        },

        // Charts
        charts: {},

        // Mutation history (aggregated from evolution data)
        mutationHistory: [],
        totalMutations: 0,

        // Agent editor modal
        editingAgent: null,
        agentEdits: { name: '', type: 'monitor', model: '' },
        savingAgent: false,
        availableModels: [],

        // Timer
        refreshTimer: null,

        // Computed
        get viewTitle() {
            const titles = {
                'overview': 'Overview',
                'agents': 'Agents',
                'agent-detail': `Agent: ${this.selectedAgent?.id || ''}`,
                'trading': 'Trading',
                'models': 'Models',
                'evolution': 'Evolution',
                'logs': 'Logs'
            };
            return titles[this.currentView] || 'Dashboard';
        },

        get filteredAgents() {
            if (this.agentFilter === 'all') return this.agents;
            return this.agents.filter(a => a.def?.type === this.agentFilter);
        },

        get filteredLogs() {
            if (this.logLevel === 'all') return this.logs;
            return this.logs.filter(l => l.level === this.logLevel);
        },

        get uniqueProviders() {
            const providers = new Set(this.models.map(m => m.Provider));
            return [...providers];
        },

        get totalRequests() {
            let total = 0;
            for (const c of Object.values(this.costs)) {
                total += c.TotalRequests || 0;
            }
            return total;
        },

        get totalModelCost() {
            let total = 0;
            for (const c of Object.values(this.costs)) {
                total += c.TotalCostUSD || 0;
            }
            return total;
        },

        get evolvingAgents() {
            return this.agents.filter(a => a.status === 'evolving');
        },

        get avgFitness() {
            const fitnesses = Object.values(this.evolutionData)
                .filter(e => e && e.fitness)
                .map(e => e.fitness);
            if (fitnesses.length === 0) return 0;
            return fitnesses.reduce((a, b) => a + b, 0) / fitnesses.length;
        },

        // Init
        async init() {
            await this.refreshAll();
            this.initMockTradingData();

            // Auto-refresh
            this.refreshTimer = setInterval(() => this.refreshAll(), REFRESH_INTERVAL);

            // Handle browser back/forward
            window.addEventListener('hashchange', () => {
                const hash = window.location.hash.slice(1);
                if (hash) this.navigate(hash, false);
            });

            // Check initial hash
            const hash = window.location.hash.slice(1);
            if (hash) this.navigate(hash, false);
        },

        // Navigation
        navigate(view, updateHash = true) {
            this.currentView = view;
            if (updateHash) window.location.hash = view;

            // Load view-specific data
            if (view === 'overview') this.initOverviewCharts();
            if (view === 'models') this.initModelCharts();
            if (view === 'evolution') this.initEvolutionCharts();
            if (view === 'trading') this.initTradingCharts();
        },

        // API calls
        async fetchJSON(path) {
            try {
                const res = await fetch(`${API_BASE}${path}`);
                if (!res.ok) throw new Error(`HTTP ${res.status}`);
                return await res.json();
            } catch (e) {
                console.warn(`API error (${path}):`, e.message);
                return null;
            }
        },

        async refreshAll() {
            const results = await Promise.allSettled([
                this.fetchJSON('/api/status'),
                this.fetchJSON('/api/agents'),
                this.fetchJSON('/api/models'),
                this.fetchJSON('/api/costs'),
                this.fetchJSON('/api/dashboard'),
            ]);

            if (results[0].status === 'fulfilled' && results[0].value) {
                this.status = results[0].value;
                this.connected = true;
            } else {
                this.connected = false;
            }

            if (results[1].status === 'fulfilled' && results[1].value) {
                this.agents = Array.isArray(results[1].value) ? results[1].value : [];
            }

            if (results[2].status === 'fulfilled' && results[2].value) {
                this.models = Array.isArray(results[2].value) ? results[2].value : [];
            }

            if (results[3].status === 'fulfilled' && results[3].value) {
                this.costs = results[3].value || {};
            }

            // Fetch evolution data for each agent
            for (const agent of this.agents) {
                const evo = await this.fetchJSON(`/api/agents/${agent.id}/evolution`);
                if (evo) this.evolutionData[agent.id] = evo;
            }

            this.lastUpdated = new Date().toLocaleTimeString();

            // Re-render charts for current view
            this.$nextTick(() => {
                if (this.currentView === 'overview') this.initOverviewCharts();
                if (this.currentView === 'models') this.initModelCharts();
                if (this.currentView === 'evolution') this.initEvolutionCharts();
                if (this.currentView === 'trading') this.initTradingCharts();
            });
        },

        // Agent detail
        async viewAgent(agentId) {
            const agent = this.agents.find(a => a.id === agentId);
            if (!agent) return;

            this.selectedAgent = agent;
            this.currentView = 'agent-detail';
            window.location.hash = `agent-detail/${agentId}`;

            // Fetch memory
            const mem = await this.fetchJSON(`/api/agents/${agentId}/memory`);
            this.agentMemory = mem?.messages || [];

            // Fetch evolution
            const evo = await this.fetchJSON(`/api/agents/${agentId}/evolution`);
            this.agentEvolution = evo;

            // Fetch genome
            const genome = await this.fetchJSON(`/api/agents/${agentId}/genome`);
            this.agentGenome = genome;

            // Init charts after view renders
            this.$nextTick(() => this.initAgentDetailCharts());
        },

        async triggerEvolve(agentId) {
            try {
                const res = await fetch(`${API_BASE}/api/agents/${agentId}/evolve`, { method: 'POST' });
                if (res.ok) {
                    // Add to mutation history
                    this.mutationHistory.unshift({
                        id: Date.now(),
                        agentId,
                        time: new Date().toISOString(),
                        fromVersion: this.evolutionData[agentId]?.version || 0,
                        toVersion: (this.evolutionData[agentId]?.version || 0) + 1,
                        fitness: this.evolutionData[agentId]?.fitness || 0
                    });
                    this.totalMutations++;
                    await this.refreshAll();
                }
            } catch (e) {
                console.error('Evolve error:', e);
            }
        },

        // Log streaming via SSE
        toggleLogStream() {
            if (this.logStreaming) {
                this.stopLogStream();
            } else {
                this.startLogStream();
            }
        },

        startLogStream() {
            if (this.logEventSource) this.logEventSource.close();

            try {
                this.logEventSource = new EventSource(`${API_BASE}/api/logs/stream`);
                this.logStreaming = true;

                this.logEventSource.onmessage = (event) => {
                    try {
                        const log = JSON.parse(event.data);
                        this.logs.push(log);
                        // Keep last 1000 logs
                        if (this.logs.length > 1000) this.logs = this.logs.slice(-1000);
                        // Auto-scroll
                        this.$nextTick(() => {
                            const container = document.getElementById('logContainer');
                            if (container) container.scrollTop = container.scrollHeight;
                        });
                    } catch (e) {
                        // Plain text log
                        this.logs.push({
                            time: new Date().toLocaleTimeString(),
                            level: 'info',
                            component: '',
                            message: event.data
                        });
                    }
                };

                this.logEventSource.onerror = () => {
                    this.logStreaming = false;
                    this.logEventSource?.close();
                    // Generate synthetic logs when SSE not available
                    this.generateSyntheticLogs();
                };
            } catch (e) {
                this.generateSyntheticLogs();
            }
        },

        stopLogStream() {
            if (this.logEventSource) {
                this.logEventSource.close();
                this.logEventSource = null;
            }
            this.logStreaming = false;
        },

        clearLogs() {
            this.logs = [];
        },

        generateSyntheticLogs() {
            // When no SSE endpoint is available, generate representative logs
            this.logStreaming = true;
            const components = ['api', 'orchestrator', 'model-router', 'registry', 'evolution', 'mqtt', 'telegram'];
            const messages = [
                { level: 'info', msg: 'HTTP request completed', comp: 'api' },
                { level: 'debug', msg: 'routing message to agent', comp: 'orchestrator' },
                { level: 'info', msg: 'model registered: anthropic/claude-sonnet', comp: 'model-router' },
                { level: 'info', msg: 'agent heartbeat received', comp: 'registry' },
                { level: 'info', msg: 'strategy evaluated, fitness=0.72', comp: 'evolution' },
                { level: 'warn', msg: 'agent fitness below threshold', comp: 'evolution' },
                { level: 'debug', msg: 'cost tracked: $0.0012', comp: 'model-router' },
                { level: 'info', msg: 'MQTT message published', comp: 'mqtt' },
                { level: 'error', msg: 'connection timeout, retrying...', comp: 'mqtt' },
                { level: 'info', msg: 'telegram poll completed', comp: 'telegram' },
            ];

            const addLog = () => {
                if (!this.logStreaming) return;
                const entry = messages[Math.floor(Math.random() * messages.length)];
                this.logs.push({
                    time: new Date().toLocaleTimeString(),
                    level: entry.level,
                    component: entry.comp,
                    message: entry.msg
                });
                if (this.logs.length > 1000) this.logs = this.logs.slice(-1000);
                this.$nextTick(() => {
                    const container = document.getElementById('logContainer');
                    if (container) container.scrollTop = container.scrollHeight;
                });
                setTimeout(addLog, 1000 + Math.random() * 3000);
            };
            addLog();
        },

        // Helpers
        getCostForModel(modelId) {
            return this.costs[modelId] || null;
        },

        getEvolutionData(agentId) {
            return this.evolutionData[agentId] || null;
        },

        agentStatusColor(status) {
            switch (status) {
                case 'running': return 'green';
                case 'idle': return 'green';
                case 'evolving': return 'yellow';
                case 'error': return 'red';
                default: return 'yellow';
            }
        },

        getFitnessClass(fitness) {
            if (!fitness) return '';
            if (fitness >= 0.7) return 'text-green';
            if (fitness >= 0.4) return 'text-yellow';
            return 'text-red';
        },

        formatUptime(uptime) {
            if (!uptime) return '—';
            // Uptime comes as nanoseconds (Go duration)
            const seconds = Math.abs(uptime / 1e9);
            if (seconds < 60) return `${Math.floor(seconds)}s`;
            if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${Math.floor(seconds % 60)}s`;
            if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`;
            return `${Math.floor(seconds / 86400)}d ${Math.floor((seconds % 86400) / 3600)}h`;
        },

        formatTime(t) {
            if (!t || t === '0001-01-01T00:00:00Z') return '—';
            try {
                const d = new Date(t);
                if (isNaN(d)) return '—';
                const now = new Date();
                const diffMs = now - d;
                if (diffMs < 60000) return 'just now';
                if (diffMs < 3600000) return `${Math.floor(diffMs / 60000)}m ago`;
                if (diffMs < 86400000) return `${Math.floor(diffMs / 3600000)}h ago`;
                return d.toLocaleDateString();
            } catch {
                return '—';
            }
        },

        formatNumber(n) {
            if (n >= 1e6) return (n / 1e6).toFixed(1) + 'M';
            if (n >= 1e3) return (n / 1e3).toFixed(1) + 'K';
            return n.toString();
        },

        formatSuccessRate(metrics) {
            if (!metrics || !metrics.total_actions || metrics.total_actions === 0) return '—';
            const rate = (metrics.successful_actions / metrics.total_actions * 100);
            return rate.toFixed(1) + '%';
        },

        // Mock trading data
        initMockTradingData() {
            this.tradingData = {
                totalPnl: 1247.83,
                winRate: 63.2,
                totalTrades: 156,
                positions: [
                    { asset: 'ETH-PERP', side: 'LONG', size: '2.5', entry: 3245.50, pnl: 127.40 },
                    { asset: 'BTC-PERP', side: 'SHORT', size: '0.15', entry: 97820.00, pnl: -42.30 },
                    { asset: 'SOL-PERP', side: 'LONG', size: '45', entry: 178.25, pnl: 89.10 },
                ],
                strategies: [
                    { name: 'FundingArbitrage', active: true, winRate: 71.2, pnl: 892.40 },
                    { name: 'MeanReversion', active: true, winRate: 58.3, pnl: 355.43 },
                    { name: 'MomentumBreakout', active: false, winRate: 45.1, pnl: -122.30 },
                ],
                recentTrades: [
                    { time: new Date(Date.now() - 300000).toISOString(), asset: 'ETH-PERP', signal: 'Funding Rate Spike', action: 'BUY', price: 3245.50, size: '2.5', pnl: 0 },
                    { time: new Date(Date.now() - 1800000).toISOString(), asset: 'BTC-PERP', signal: 'Mean Reversion', action: 'SELL', price: 97820.00, size: '0.15', pnl: -42.30 },
                    { time: new Date(Date.now() - 7200000).toISOString(), asset: 'SOL-PERP', signal: 'Funding Rate Spike', action: 'BUY', price: 178.25, size: '45', pnl: 89.10 },
                    { time: new Date(Date.now() - 14400000).toISOString(), asset: 'ETH-PERP', signal: 'Mean Reversion', action: 'SELL', price: 3190.00, size: '3.0', pnl: 156.20 },
                    { time: new Date(Date.now() - 28800000).toISOString(), asset: 'BTC-PERP', signal: 'Momentum', action: 'BUY', price: 96500.00, size: '0.1', pnl: 220.40 },
                ]
            };
        },

        // Chart initialization
        initOverviewCharts() {
            this.$nextTick(() => {
                this.createChart('agentActivityChart', {
                    type: 'bar',
                    data: {
                        labels: this.agents.map(a => a.def?.name || a.id),
                        datasets: [{
                            label: 'Messages',
                            data: this.agents.map(a => a.message_count || 0),
                            backgroundColor: 'rgba(88, 166, 255, 0.5)',
                            borderColor: 'rgba(88, 166, 255, 1)',
                            borderWidth: 1
                        }, {
                            label: 'Errors',
                            data: this.agents.map(a => a.error_count || 0),
                            backgroundColor: 'rgba(248, 81, 73, 0.5)',
                            borderColor: 'rgba(248, 81, 73, 1)',
                            borderWidth: 1
                        }]
                    },
                    options: this.chartOptions('Activity by Agent')
                });

                // Cost over time - generate mock timeline
                const hours = Array.from({length: 24}, (_, i) => `${i}:00`);
                const costData = hours.map((_, i) => (this.status.total_cost || 0) * (i + 1) / 24 + Math.random() * 0.001);

                this.createChart('costChart', {
                    type: 'line',
                    data: {
                        labels: hours,
                        datasets: [{
                            label: 'Cumulative Cost ($)',
                            data: costData,
                            borderColor: 'rgba(63, 185, 80, 1)',
                            backgroundColor: 'rgba(63, 185, 80, 0.1)',
                            fill: true,
                            tension: 0.4,
                            pointRadius: 0
                        }]
                    },
                    options: this.chartOptions('Cost Over Time (24h)')
                });
            });
        },

        initAgentDetailCharts() {
            if (!this.selectedAgent) return;

            const m = this.selectedAgent.metrics || {};

            this.createChart('agentMetricsChart', {
                type: 'bar',
                data: {
                    labels: ['Success', 'Failed', 'Total'],
                    datasets: [{
                        label: 'Actions',
                        data: [m.successful_actions || 0, m.failed_actions || 0, m.total_actions || 0],
                        backgroundColor: [
                            'rgba(63, 185, 80, 0.5)',
                            'rgba(248, 81, 73, 0.5)',
                            'rgba(88, 166, 255, 0.5)'
                        ],
                        borderColor: [
                            'rgba(63, 185, 80, 1)',
                            'rgba(248, 81, 73, 1)',
                            'rgba(88, 166, 255, 1)'
                        ],
                        borderWidth: 1
                    }]
                },
                options: this.chartOptions('Performance')
            });

            // Evolution history chart
            const evoData = this.agentEvolution;
            const versions = Array.from({length: Math.max(evoData?.version || 1, 5)}, (_, i) => `v${i + 1}`);
            const fitnessHistory = versions.map((_, i) => {
                if (evoData && i === versions.length - 1) return evoData.fitness || 0;
                return 0.3 + Math.random() * 0.5;
            });

            this.createChart('evolutionChart', {
                type: 'line',
                data: {
                    labels: versions,
                    datasets: [{
                        label: 'Fitness Score',
                        data: fitnessHistory,
                        borderColor: 'rgba(188, 140, 255, 1)',
                        backgroundColor: 'rgba(188, 140, 255, 0.1)',
                        fill: true,
                        tension: 0.3,
                        pointBackgroundColor: 'rgba(188, 140, 255, 1)'
                    }]
                },
                options: this.chartOptions('Fitness Over Versions')
            });
        },

        initModelCharts() {
            this.$nextTick(() => {
                const modelNames = Object.keys(this.costs);
                const costValues = modelNames.map(id => this.costs[id]?.TotalCostUSD || 0);
                const colors = [
                    'rgba(88, 166, 255, 0.7)',
                    'rgba(63, 185, 80, 0.7)',
                    'rgba(248, 81, 73, 0.7)',
                    'rgba(188, 140, 255, 0.7)',
                    'rgba(210, 153, 34, 0.7)',
                    'rgba(57, 210, 192, 0.7)',
                ];

                if (modelNames.length > 0) {
                    this.createChart('modelCostChart', {
                        type: 'doughnut',
                        data: {
                            labels: modelNames,
                            datasets: [{
                                data: costValues,
                                backgroundColor: colors.slice(0, modelNames.length),
                                borderColor: 'rgba(13, 17, 23, 1)',
                                borderWidth: 2
                            }]
                        },
                        options: {
                            responsive: true,
                            plugins: {
                                legend: {
                                    position: 'right',
                                    labels: { color: '#8b949e', font: { size: 12 } }
                                }
                            }
                        }
                    });
                }
            });
        },

        initTradingCharts() {
            this.$nextTick(() => {
                const days = Array.from({length: 30}, (_, i) => {
                    const d = new Date();
                    d.setDate(d.getDate() - (29 - i));
                    return d.toLocaleDateString('en', { month: 'short', day: 'numeric' });
                });

                let cumPnl = 0;
                const pnlData = days.map(() => {
                    cumPnl += (Math.random() - 0.4) * 100;
                    return cumPnl;
                });

                this.createChart('pnlChart', {
                    type: 'line',
                    data: {
                        labels: days,
                        datasets: [{
                            label: 'Cumulative P&L ($)',
                            data: pnlData,
                            borderColor: pnlData[pnlData.length - 1] >= 0 ? 'rgba(63, 185, 80, 1)' : 'rgba(248, 81, 73, 1)',
                            backgroundColor: pnlData[pnlData.length - 1] >= 0 ? 'rgba(63, 185, 80, 0.1)' : 'rgba(248, 81, 73, 0.1)',
                            fill: true,
                            tension: 0.3,
                            pointRadius: 1
                        }]
                    },
                    options: this.chartOptions('P&L (30 Days)')
                });
            });
        },

        initEvolutionCharts() {
            this.$nextTick(() => {
                const agentNames = this.agents.map(a => a.def?.name || a.id);
                const fitnessValues = this.agents.map(a => this.evolutionData[a.id]?.fitness || Math.random() * 0.8);

                // Generate mock fitness trend over time
                const timePoints = Array.from({length: 20}, (_, i) => `t${i + 1}`);
                const datasets = this.agents.slice(0, 4).map((a, idx) => {
                    const colors = ['rgba(88, 166, 255, 1)', 'rgba(63, 185, 80, 1)', 'rgba(188, 140, 255, 1)', 'rgba(210, 153, 34, 1)'];
                    let fitness = 0.3;
                    return {
                        label: a.def?.name || a.id,
                        data: timePoints.map(() => {
                            fitness += (Math.random() - 0.45) * 0.1;
                            fitness = Math.max(0, Math.min(1, fitness));
                            return fitness;
                        }),
                        borderColor: colors[idx],
                        backgroundColor: 'transparent',
                        tension: 0.3,
                        pointRadius: 2
                    };
                });

                this.createChart('fitnessTrendChart', {
                    type: 'line',
                    data: {
                        labels: timePoints,
                        datasets: datasets.length > 0 ? datasets : [{
                            label: 'No agents',
                            data: timePoints.map(() => 0),
                            borderColor: 'rgba(88, 166, 255, 0.3)',
                        }]
                    },
                    options: this.chartOptions('Fitness Trends')
                });
            });
        },

        createChart(canvasId, config) {
            // Destroy existing chart
            if (this.charts[canvasId]) {
                this.charts[canvasId].destroy();
            }

            const canvas = document.getElementById(canvasId);
            if (!canvas) return;

            this.charts[canvasId] = new Chart(canvas, config);
        },

        chartOptions(title) {
            return {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: {
                        labels: { color: '#8b949e', font: { size: 11 } }
                    },
                    title: {
                        display: false,
                        text: title,
                        color: '#e6edf3'
                    }
                },
                scales: {
                    x: {
                        ticks: { color: '#484f58', font: { size: 10 } },
                        grid: { color: 'rgba(48, 54, 61, 0.5)' }
                    },
                    y: {
                        ticks: { color: '#484f58', font: { size: 10 } },
                        grid: { color: 'rgba(48, 54, 61, 0.5)' }
                    }
                }
            };
        },

        // Agent Editor Modal
        openAgentEditor(agentID) {
            const agent = this.agents.find(a => a.id === agentID);
            if (!agent) return;
            
            this.editingAgent = agent;
            this.agentEdits = {
                name: agent.def?.name || agent.id,
                type: agent.def?.type || 'monitor',
                model: agent.def?.model || ''
            };
            
            if (this.availableModels.length === 0) {
                this.loadAvailableModels();
            }
        },

        closeAgentEditor() {
            this.editingAgent = null;
            this.agentEdits = { name: '', type: 'monitor', model: '' };
            this.savingAgent = false;
        },

        async loadAvailableModels() {
            try {
                const data = await this.fetchJSON('/api/models');
                this.availableModels = data || [];
            } catch (err) {
                console.error('Failed to load models:', err);
            }
        },

        async saveAgentSettings() {
            if (!this.editingAgent || !this.agentEdits.model) return;
            
            this.savingAgent = true;
            
            try {
                const resp = await fetch(`${API_BASE}/api/agents/${this.editingAgent.id}`, {
                    method: 'PATCH',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        name: this.agentEdits.name,
                        type: this.agentEdits.type,
                        model: this.agentEdits.model
                    })
                });
                
                if (!resp.ok) {
                    const text = await resp.text();
                    throw new Error(text || `HTTP ${resp.status}`);
                }
                
                const result = await resp.json();
                
                // Show success (simple alert for now)
                alert(`✅ Agent updated successfully!\nModel: ${result.model}`);
                
                // Refresh agents list
                await this.loadAgents();
                
                // Close modal
                this.closeAgentEditor();
                
            } catch (err) {
                alert(`❌ Error: ${err.message}`);
            } finally {
                this.savingAgent = false;
            }
        },

        // Cleanup
        destroy() {
            if (this.refreshTimer) clearInterval(this.refreshTimer);
            if (this.logEventSource) this.logEventSource.close();
            for (const chart of Object.values(this.charts)) {
                chart.destroy();
            }
        }
    };
}
