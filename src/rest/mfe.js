class WatchdogAgentMFE extends HTMLElement {
    constructor() {
        super();
        this.baseUrl = '';
        this.statusData = { healthy: false, status: 'Offline', version: 'N/A', timestamp: 0, uptime_seconds: 0, services: [] };
    }

    async connectedCallback() {
        this.baseUrl = this.getAttribute('base-url') || 'http://localhost:9095';
        this.renderSkeleton();
        await this.loadAll();
        // Auto refresh status every 5 seconds
        this.refreshInterval = setInterval(() => this.loadStatus(), 5000);
    }

    disconnectedCallback() {
        if (this.refreshInterval) {
            clearInterval(this.refreshInterval);
        }
    }

    renderSkeleton() {
        this.innerHTML = `
            <style>
                .mfe-container {
                    font-family: var(--font-sans, 'Inter', sans-serif);
                    color: var(--color-text-primary, #e2e8f0);
                    max-width: 1100px;
                    margin: 0 auto;
                    padding: 20px;
                }
                .mfe-header {
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    margin-bottom: 24px;
                }
                .mfe-title h1 {
                    font-family: var(--font-display, 'Outfit', sans-serif);
                    font-size: 2rem;
                    font-weight: 700;
                    margin: 0 0 5px 0;
                    color: var(--color-text-primary, white);
                }
                .mfe-title p {
                    color: var(--color-text-secondary, #94a3b8);
                    margin: 0;
                    font-size: 0.95rem;
                }
                .mfe-btn {
                    padding: 10px 18px;
                    border-radius: 8px;
                    font-weight: 600;
                    font-size: 0.9rem;
                    cursor: pointer;
                    transition: all 0.2s;
                    border: none;
                    display: inline-flex;
                    align-items: center;
                    gap: 6px;
                }
                .mfe-btn-danger {
                    background: linear-gradient(135deg, #f56565 0%, #c53030 100%);
                    color: white;
                }
                .mfe-btn-primary {
                    background: linear-gradient(135deg, #3182ce 0%, #2b6cb0 100%);
                    color: white;
                }
                .mfe-btn-secondary {
                    background-color: rgba(255, 255, 255, 0.05);
                    color: #e2e8f0;
                    border: 1px solid rgba(255, 255, 255, 0.1);
                }
                .mfe-btn-secondary:hover {
                    background-color: rgba(255, 255, 255, 0.1);
                }
                .mfe-btn:hover {
                    opacity: 0.95;
                    transform: translateY(-1px);
                }
                
                /* Cards layout */
                .mfe-row {
                    display: flex;
                    gap: 20px;
                    margin-bottom: 24px;
                    flex-wrap: wrap;
                }
                .mfe-col-4 { flex: 1; min-width: 250px; }
                .mfe-col-8 { flex: 2; min-width: 320px; }
                
                .mfe-card {
                    background: rgba(30, 41, 59, 0.7);
                    backdrop-filter: blur(10px);
                    border-radius: 12px;
                    padding: 20px;
                    border: 1px solid rgba(255, 255, 255, 0.05);
                }
                .mfe-card-title {
                    font-size: 0.75rem;
                    text-transform: uppercase;
                    color: #94a3b8;
                    font-weight: 700;
                    margin-bottom: 12px;
                    letter-spacing: 0.05em;
                }
                .mfe-status-indicator {
                    display: flex;
                    align-items: center;
                    gap: 8px;
                    font-size: 1.25rem;
                    font-weight: 700;
                    margin-bottom: 10px;
                    color: white;
                }
                .mfe-dot {
                    height: 10px;
                    width: 10px;
                    border-radius: 50%;
                    display: inline-block;
                }
                .mfe-dot-active {
                    background-color: #48bb78;
                    box-shadow: 0 0 8px #48bb78;
                }
                .mfe-dot-completed {
                    background-color: #4299e1;
                    box-shadow: 0 0 8px #4299e1;
                }
                .mfe-dot-inactive {
                    background-color: #f56565;
                    box-shadow: 0 0 8px #f56565;
                }
                
                .mfe-badge {
                    background-color: rgba(66, 153, 225, 0.1);
                    color: #63b3ed;
                    border: 1px solid rgba(66, 153, 225, 0.2);
                    padding: 2px 6px;
                    border-radius: 4px;
                    font-size: 0.7rem;
                    font-weight: 600;
                    margin-right: 4px;
                    display: inline-block;
                }
                
                /* Services table list */
                .mfe-table-container {
                    background: rgba(30, 41, 59, 0.7);
                    backdrop-filter: blur(10px);
                    border-radius: 12px;
                    border: 1px solid rgba(255, 255, 255, 0.05);
                    overflow: hidden;
                }
                .mfe-table-header {
                    background: rgba(15, 23, 42, 0.5);
                    padding: 20px;
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    border-bottom: 1px solid rgba(255, 255, 255, 0.05);
                }
                .mfe-table-header h2 {
                    font-family: var(--font-display, 'Outfit', sans-serif);
                    font-size: 1.25rem;
                    margin: 0;
                    font-weight: 700;
                    color: white;
                }
                
                .mfe-table {
                    width: 100%;
                    border-collapse: collapse;
                    text-align: left;
                }
                .mfe-table th {
                    background: rgba(15, 23, 42, 0.3);
                    color: #94a3b8;
                    padding: 12px 20px;
                    font-size: 0.75rem;
                    text-transform: uppercase;
                    font-weight: 700;
                    border-bottom: 1px solid rgba(255, 255, 255, 0.05);
                }
                .mfe-table td {
                    padding: 14px 20px;
                    border-bottom: 1px solid rgba(255, 255, 255, 0.05);
                    font-size: 0.9rem;
                    vertical-align: middle;
                }
                .mfe-mono {
                    font-family: var(--font-mono, monospace);
                    font-weight: 600;
                }
                .mfe-actions {
                    text-align: right;
                }
            </style>
            
            <div class="mfe-container">
                <!-- Header -->
                <div class="mfe-header">
                    <div class="mfe-title">
                        <h1>🤖 Watchdog Supervisor</h1>
                        <p>OpenMFE dynamic service supervisor and live daemon processes health dashboard</p>
                    </div>
                    <button class="mfe-btn mfe-btn-danger" id="mfe-restart-all-btn">
                        <i class="fa fa-refresh"></i> Restart All Services
                    </button>
                </div>
                
                <!-- Status Row -->
                <div class="mfe-row">
                    <div class="mfe-col-4">
                        <div class="mfe-card">
                            <div class="mfe-card-title">Watchdog Status</div>
                            <div class="mfe-status-indicator">
                                <span class="mfe-dot mfe-dot-inactive" id="mfe-status-dot"></span>
                                <span id="mfe-status-text">Offline</span>
                            </div>
                            <div style="color:#94a3b8; font-size:0.85rem;" id="mfe-status-details">
                                Loading supervisor...
                            </div>
                        </div>
                    </div>
                    <div class="mfe-col-8">
                        <div class="mfe-card" style="height:100%; box-sizing:border-box; display: flex; flex-direction: column; justify-content: center;">
                            <div class="mfe-card-title">Ecosystem Telemetry</div>
                            <div style="font-size:0.95rem; line-height: 1.5;" id="mfe-stats-details">
                                Loading stats...
                            </div>
                        </div>
                    </div>
                </div>
                
                <!-- Services Table -->
                <div class="mfe-table-container">
                    <div class="mfe-table-header">
                        <h2><i class="fa fa-cogs"></i> Supervised Fleet Processes</h2>
                    </div>
                    <div style="overflow-x:auto;">
                        <table class="mfe-table">
                            <thead>
                                <tr>
                                    <th style="width:25%;">Service Name</th>
                                    <th style="width:25%;">Status</th>
                                    <th style="width:15%;">PID</th>
                                    <th style="width:25%;">Dependencies</th>
                                    <th style="width:10%; text-align:right;">Actions</th>
                                </tr>
                            </thead>
                            <tbody id="mfe-services-list">
                                <tr>
                                    <td colspan="5" style="padding:40px; text-align:center; color:#94a3b8;">
                                        <i class="fa fa-spinner fa-spin fa-2x"></i><br>Querying process registry...
                                    </td>
                                </tr>
                            </tbody>
                        </table>
                    </div>
                </div>
            </div>
        `;

        this.bindEvents();
    }

    bindEvents() {
        this.querySelector('#mfe-restart-all-btn').addEventListener('click', () => this.restartAll());
    }

    async loadAll() {
        await this.loadStatus();
    }

    async loadStatus() {
        try {
            const res = await fetch(`${this.baseUrl}/api/v1/status`);
            if (res.ok) {
                this.statusData = await res.json();
                this.displayStatus();
                this.displayServices();
            } else {
                throw new Error('Unreachable status');
            }
        } catch (err) {
            console.error('Failed to load status:', err);
            this.statusData.healthy = false;
            this.statusData.status = 'Error';
            this.displayStatus();
            this.querySelector('#mfe-services-list').innerHTML = `
                <tr>
                    <td colspan="5" style="padding:40px; text-align:center; color:#f56565;">
                        <i class="fa fa-exclamation-triangle fa-2x"></i><br>Failed to retrieve processes from watchdog daemon.
                    </td>
                </tr>
            `;
        }
    }

    displayStatus() {
        const dot = this.querySelector('#mfe-status-dot');
        const text = this.querySelector('#mfe-status-text');
        const details = this.querySelector('#mfe-status-details');
        const stats = this.querySelector('#mfe-stats-details');

        if (dot && text) {
            if (this.statusData.healthy) {
                dot.className = 'mfe-dot mfe-dot-active';
                text.innerText = ' Operational';
            } else {
                dot.className = 'mfe-dot mfe-dot-inactive';
                text.innerText = ` ${this.statusData.status}`;
            }
        }

        const date = new Date(this.statusData.timestamp * 1000).toLocaleString();
        details.innerHTML = `
            <div><strong>Version:</strong> ${this.escapeHTML(this.statusData.version || 'N/A')}</div>
            <div><strong>Last Check:</strong> ${date}</div>
        `;

        if (this.statusData.healthy) {
            const runningCount = this.statusData.services.filter(s => s.running).length;
            const totalCount = this.statusData.services.length;

            const pgDot = this.statusData.postgres_connected ? 'mfe-dot-active' : 'mfe-dot-inactive';
            const pgStatus = this.statusData.postgres_connected ? 'Connected' : 'Disconnected';
            const mcpDot = this.statusData.rag_mcp_connected ? 'mfe-dot-active' : 'mfe-dot-inactive';
            const mcpStatus = this.statusData.rag_mcp_connected ? 'Connected' : 'Disconnected';

            stats.innerHTML = `
                <div style="display:flex; justify-content:space-between; flex-wrap:wrap; gap:16px; align-items:center; width:100%;">
                    <div>
                        <div><strong>Watchdog Uptime:</strong> <span class="mfe-mono">${this.statusData.uptime_seconds} seconds</span></div>
                        <div><strong>Managed Services:</strong> <span class="mfe-mono">${runningCount} active / ${totalCount} total</span></div>
                    </div>
                    <div style="border-left: 1px solid rgba(255,255,255,0.1); padding-left:24px; min-width:240px;">
                        <div style="display:flex; align-items:center; gap:8px;">
                            <strong>Postgres DB:</strong>
                            <span class="mfe-dot ${pgDot}"></span>
                            <span>${pgStatus} <span style="font-size:0.75rem; color:#94a3b8;">(${this.escapeHTML(this.statusData.postgres_addr)})</span></span>
                        </div>
                        <div style="display:flex; align-items:center; gap:8px; margin-top:6px;">
                            <strong>RAG MCP Server:</strong>
                            <span class="mfe-dot ${mcpDot}"></span>
                            <span>${mcpStatus} <span style="font-size:0.75rem; color:#94a3b8;">(${this.escapeHTML(this.statusData.rag_mcp_addr)})</span></span>
                        </div>
                    </div>
                </div>
            `;
        } else {
            stats.innerHTML = `<span style="color:#f56565;">Supervisor connection offline.</span>`;
        }
    }

    displayServices() {
        const list = this.querySelector('#mfe-services-list');
        list.innerHTML = '';

        if (!this.statusData.services || this.statusData.services.length === 0) {
            list.innerHTML = `
                <tr>
                    <td colspan="5" style="padding:40px; text-align:center; color:#94a3b8;">
                        No supervised services configured.
                    </td>
                </tr>
            `;
            return;
        }

        this.statusData.services.forEach(svc => {
            const row = document.createElement('tr');

            let dotClass = 'mfe-dot mfe-dot-inactive';
            let stateText = 'Stopped';
            if (svc.running) {
                dotClass = 'mfe-dot mfe-dot-active';
                stateText = 'Running';
            } else if (svc.one_shot && svc.completed) {
                dotClass = 'mfe-dot mfe-dot-completed';
                stateText = 'Completed';
            } else if (svc.one_shot && svc.failed) {
                dotClass = 'mfe-dot mfe-dot-inactive';
                stateText = 'Failed';
            }

            const pidVal = svc.running && svc.pid ? svc.pid : '-';
            const depBadges = svc.deps && svc.deps.length > 0
                ? svc.deps.map(d => `<span class="mfe-badge">${this.escapeHTML(d)}</span>`).join('')
                : '<span style="color:#64748b; font-size:0.8rem;">None</span>';

            const isExternal = svc.name === "timescale-db" || svc.name === "rag-mcp";
            let actionHTML = '';
            if (isExternal) {
                if (svc.name === "timescale-db" && !svc.running) {
                    actionHTML = `<button class="mfe-btn mfe-btn-primary mfe-start-db-btn" style="padding: 6px 12px; font-size:0.8rem;">
                            <i class="fa fa-play"></i> Start DB
                        </button>`;
                } else {
                    actionHTML = `<span style="color:#64748b; font-size:0.85rem; font-style:italic; padding-right:12px;">External</span>`;
                }
            } else {
                actionHTML = `<button class="mfe-btn mfe-btn-secondary mfe-restart-svc-btn" data-name="${this.escapeAttribute(svc.name)}" style="padding: 6px 12px; font-size:0.8rem;">
                        <i class="fa fa-refresh"></i> Restart
                    </button>`;
            }

            row.innerHTML = `
                <td class="mfe-mono" style="font-weight:700; color:white;">${this.escapeHTML(svc.name)}</td>
                <td>
                    <span style="display:flex; align-items:center; gap:8px;">
                        <span class="${dotClass}"></span>
                        <span>${stateText}</span>
                    </span>
                </td>
                <td class="mfe-mono">${pidVal}</td>
                <td>${depBadges}</td>
                <td class="mfe-actions">${actionHTML}</td>
            `;

            // Bind click event handlers
            if (isExternal) {
                if (svc.name === "timescale-db" && !svc.running) {
                    row.querySelector('.mfe-start-db-btn').addEventListener('click', () => {
                        this.startPostgres();
                    });
                }
            } else {
                row.querySelector('.mfe-restart-svc-btn').addEventListener('click', (e) => {
                    const name = e.currentTarget.dataset.name;
                    this.restartService(name);
                });
            }

            list.appendChild(row);
        });
    }

    async startPostgres() {
        if (!confirm("Are you sure you want to attempt starting the Postgres database?")) return;
        try {
            const res = await fetch(`${this.baseUrl}/api/v1/postgres/start`, {
                method: 'POST'
            });
            if (res.ok) {
                alert("Postgres start process initiated in the background.");
                await this.loadStatus();
            } else {
                const text = await res.text();
                alert(`Error launching database: ${text}`);
            }
        } catch (err) {
            alert(`Failed to trigger database launch: ${err.message}`);
        }
    }

    async restartService(name) {
        if (!confirm(`Are you sure you want to restart service '${name}'?`)) return;
        try {
            const res = await fetch(`${this.baseUrl}/api/v1/watchdog/restart`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ name })
            });
            if (res.ok) {
                alert(`Restart triggered for service '${name}'.`);
                await this.loadStatus();
            } else {
                const text = await res.text();
                alert(`Error restarting service: ${text}`);
            }
        } catch (err) {
            alert(`Failed to restart service: ${err.message}`);
        }
    }

    async restartAll() {
        if (!confirm("Are you sure you want to restart ALL managed ecosystem services? This will cleanly shut down and rebuild everything sequentially!")) return;
        try {
            const res = await fetch(`${this.baseUrl}/api/v1/watchdog/restart_all`, { method: 'POST' });
            if (res.ok) {
                alert("Restart sequence triggered for all services.");
                await this.loadStatus();
            } else {
                alert("Restart all failed.");
            }
        } catch (err) {
            alert(`Failed to trigger restart all: ${err.message}`);
        }
    }

    escapeHTML(value) {
        if (value === null || value === undefined) return '';
        return String(value).replace(/[&<>"']/g, char => ({
            '&': '&amp;',
            '<': '&lt;',
            '>': '&gt;',
            '"': '&quot;',
            "'": '&#39;'
        }[char]));
    }

    escapeAttribute(value) {
        return this.escapeHTML(value);
    }
}

customElements.define('watchdog-agent-mfe', WatchdogAgentMFE);
