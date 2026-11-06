// =============================================================================
// ESSENTIAL PROCESS:
// OpenMFE web component for watchdog-agent, encapsulating supervisor status,
// managed processes telemetry, restart actions, and database initialization.
//
// DATA FLOW:
// 1. Input: Service and process states fetched via HTTP from /api/v1/status.
// 2. Logic: Renders responsive dashboard, interactive tables, and action triggers.
// 3. Output: Dispatches HTTP REST restart/start mutations to watchdog-agent daemon.
//
// KEY PARAMETERS:
// - WatchdogAgentMFE: Standard OpenMFE Web Component registered as watchdog-agent-mfe.
// - Complies strictly with Bastien UI tokens and openmfe-components.css.
// =============================================================================

(function () {
    'use strict';

const BaseElementClass = (typeof window !== 'undefined' && window.BastienMFEElement)
    ? window.BastienMFEElement
    : class extends (typeof HTMLElement !== 'undefined' ? HTMLElement : class {}) {
        constructor() {
            super();
            this.baseUrl = '';
            this.pollTimers = [];
            this.dom = {};
            this.state = 'IDLE';
        }
        connectedCallback() {
            let origin = this.getAttribute('base-url') || (typeof window !== 'undefined' && window.location ? window.location.origin : '');
            if (origin.endsWith('/')) origin = origin.slice(0, -1);
            this.baseUrl = origin;
            this.render();
            this.cacheDom();
            this.bindEvents();
            this.onMount();
        }
        disconnectedCallback() {
            this.clearPollTimers();
            this.onUnmount();
        }
        render() {}
        cacheDom() {}
        bindEvents() {}
        onMount() {}
        onUnmount() {}
        registerPoll(cb, ms = 5000, imm = true) {
            if (imm) { try { cb(); } catch (e) { console.error(e); } }
            if (!this.pollTimers) this.pollTimers = [];
            const t = setInterval(cb, ms);
            this.pollTimers.push(t);
            return t;
        }
        clearPollTimers() {
            if (this.pollTimers) this.pollTimers.forEach(t => clearInterval(t));
            this.pollTimers = [];
        }
    };

class WatchdogAgentMFE extends BaseElementClass {
    constructor() {
        super();
        this.statusData = { healthy: false, status: 'Offline', version: 'N/A', timestamp: 0, uptime_seconds: 0, services: [] };
    }

    render() {
        this.innerHTML = `
            <div class="openmfe-container">
                <!-- Header Toolbar -->
                <div class="openmfe-toolbar">
                    <div class="openmfe-toolbar__title">
                        <i class="fa fa-shield" style="color: var(--color-accent-primary, #38bdf8);"></i>
                        <span>Watchdog Process Supervisor</span>
                        <span class="openmfe-badge" id="mfe-status-badge">
                            <span class="openmfe-dot openmfe-dot--inactive" id="mfe-status-dot"></span>
                            <span id="mfe-status-text">Offline</span>
                        </span>
                    </div>
                    <div class="openmfe-toolbar__actions">
                        <button class="openmfe-btn openmfe-btn--sm openmfe-btn--danger" id="mfe-restart-all-btn" title="Sequential full fleet restart">
                            <i class="fa fa-refresh"></i> Restart All Services
                        </button>
                    </div>
                </div>
                
                <!-- Status Row -->
                <div class="openmfe-kpi-grid">
                    <!-- Watchdog Supervisor Health -->
                    <div class="openmfe-card">
                        <div class="openmfe-card__header">
                            <h4 class="openmfe-card__title"><i class="fa fa-heartbeat"></i> Supervisor Health</h4>
                        </div>
                        <div class="openmfe-card__body">
                            <div style="display:flex; align-items:center; gap:8px; font-size:1.1rem; font-weight:700; margin-bottom:8px;">
                                <span class="openmfe-dot openmfe-dot--inactive" id="mfe-status-dot-inner"></span>
                                <span id="mfe-status-text-inner">Offline</span>
                            </div>
                            <div style="color:var(--color-text-muted, #94a3b8); font-size:0.8125rem; line-height:1.6;" id="mfe-status-details">
                                Loading supervisor...
                            </div>
                        </div>
                    </div>

                    <!-- Ecosystem Telemetry -->
                    <div class="openmfe-card">
                        <div class="openmfe-card__header">
                            <h4 class="openmfe-card__title"><i class="fa fa-server"></i> Fleet Infrastructure</h4>
                        </div>
                        <div class="openmfe-card__body" style="display:flex; flex-direction:column; justify-content:center;">
                            <div style="font-size:0.875rem; line-height: 1.6;" id="mfe-stats-details">
                                Loading stats...
                            </div>
                        </div>
                    </div>
                </div>
                
                <!-- Services Table -->
                <div class="openmfe-card" style="margin-top: var(--space-4, 16px);">
                    <div class="openmfe-card__header">
                        <h4 class="openmfe-card__title"><i class="fa fa-cogs"></i> Supervised Fleet Processes</h4>
                    </div>
                    <div class="openmfe-card__body" style="padding: 0; overflow-x: auto;">
                        <table class="openmfe-table">
                            <thead>
                                <tr>
                                    <th style="width:25%;">Service Name</th>
                                    <th style="width:22%;">Status</th>
                                    <th style="width:15%;">PID</th>
                                    <th style="width:26%;">Dependencies</th>
                                    <th style="width:12%; text-align:right;">Actions</th>
                                </tr>
                            </thead>
                            <tbody id="mfe-services-list">
                                <tr>
                                    <td colspan="5" style="padding:40px; text-align:center; color:var(--color-text-muted, #94a3b8);">
                                        <i class="fa fa-spinner fa-spin fa-2x"></i><br><br>Querying process registry...
                                    </td>
                                </tr>
                            </tbody>
                        </table>
                    </div>
                </div>
            </div>
        `;
    }

    cacheDom() {
        this.dom = {
            statusDot: this.querySelector('#mfe-status-dot'),
            statusDotInner: this.querySelector('#mfe-status-dot-inner'),
            statusText: this.querySelector('#mfe-status-text'),
            statusTextInner: this.querySelector('#mfe-status-text-inner'),
            statusDetails: this.querySelector('#mfe-status-details'),
            statsDetails: this.querySelector('#mfe-stats-details'),
            servicesList: this.querySelector('#mfe-services-list'),
            btnRestartAll: this.querySelector('#mfe-restart-all-btn')
        };
    }

    bindEvents() {
        this.dom.btnRestartAll?.addEventListener('click', () => this.restartAll());
    }

    async onMount() {
        await this.loadAll();
        this.registerPoll(() => this.loadStatus(), 5000, false);
    }

    async loadAll() {
        await this.loadStatus();
    }

    async loadStatus() {
        try {
            let url = `${this.baseUrl}/api/v1/status`;
            let res = await fetch(url).catch(() => null);
            if ((!res || !res.ok) && typeof window !== 'undefined' && this.baseUrl !== window.location.origin) {
                res = await fetch(`${window.location.origin}/api/v1/status`).catch(() => null);
            }
            if (res && res.ok) {
                this.statusData = await res.json();
                this.displayStatus();
                this.displayServices();
            } else {
                throw new Error('Unreachable status');
            }
        } catch (err) {
            this.statusData.healthy = false;
            this.statusData.status = 'Error';
            this.displayStatus();
            if (this.dom.servicesList) {
                this.dom.servicesList.innerHTML = `
                    <tr>
                        <td colspan="5" style="padding:40px; text-align:center; color:var(--color-danger, #fb7185);">
                            <i class="fa fa-exclamation-triangle fa-2x"></i><br><br>Failed to retrieve processes from watchdog daemon: ${this.escapeHTML(err.message)}
                        </td>
                    </tr>
                `;
            }
        }
    }

    displayStatus() {
        const isOnline = this.statusData.healthy;
        const statusLabel = isOnline ? 'Operational' : ` ${this.statusData.status || 'Offline'}`;
        const dotClass = `openmfe-dot ${isOnline ? 'openmfe-dot--active' : 'openmfe-dot--inactive'}`;

        if (this.dom.statusDot) this.dom.statusDot.className = dotClass;
        if (this.dom.statusDotInner) this.dom.statusDotInner.className = dotClass;
        if (this.dom.statusText) this.dom.statusText.innerText = statusLabel;
        if (this.dom.statusTextInner) this.dom.statusTextInner.innerText = statusLabel;

        const date = this.statusData.timestamp ? new Date(this.statusData.timestamp * 1000).toLocaleString() : '—';
        if (this.dom.statusDetails) {
            this.dom.statusDetails.innerHTML = `
                <div><strong>Version:</strong> ${this.escapeHTML(this.statusData.version || 'N/A')}</div>
                <div><strong>Last Check:</strong> ${date}</div>
            `;
        }

        if (this.dom.statsDetails) {
            if (this.statusData.healthy) {
                const runningCount = (this.statusData.services || []).filter(s => s.running).length;
                const totalCount = (this.statusData.services || []).length;

                const pgDot = this.statusData.postgres_connected ? 'openmfe-dot--active' : 'openmfe-dot--inactive';
                const pgStatus = this.statusData.postgres_connected ? 'Connected' : 'Disconnected';
                const mcpDot = this.statusData.rag_mcp_connected ? 'openmfe-dot--active' : 'openmfe-dot--inactive';
                const mcpStatus = this.statusData.rag_mcp_connected ? 'Connected' : 'Disconnected';

                this.dom.statsDetails.innerHTML = `
                    <div style="display:flex; justify-content:space-between; flex-wrap:wrap; gap:16px; align-items:center; width:100%;">
                        <div>
                            <div><strong>Watchdog Uptime:</strong> <span style="font-family:var(--font-mono, monospace);">${this.statusData.uptime_seconds || 0} seconds</span></div>
                            <div style="margin-top:4px;"><strong>Managed Services:</strong> <span style="font-family:var(--font-mono, monospace); font-weight:600; color:var(--color-accent-primary, #38bdf8);">${runningCount} active</span> / ${totalCount} total</div>
                        </div>
                        <div style="border-left: 1px solid var(--color-border-subtle, rgba(255,255,255,0.08)); padding-left:20px; min-width:220px;">
                            <div style="display:flex; align-items:center; gap:8px;">
                                <strong>Postgres DB:</strong>
                                <span class="openmfe-dot ${pgDot}"></span>
                                <span>${pgStatus} <span style="font-size:0.75rem; color:var(--color-text-muted, #94a3b8);">(${this.escapeHTML(this.statusData.postgres_addr || '5432')})</span></span>
                            </div>
                            <div style="display:flex; align-items:center; gap:8px; margin-top:6px;">
                                <strong>RAG MCP Server:</strong>
                                <span class="openmfe-dot ${mcpDot}"></span>
                                <span>${mcpStatus} <span style="font-size:0.75rem; color:var(--color-text-muted, #94a3b8);">(${this.escapeHTML(this.statusData.rag_mcp_addr || '8090')})</span></span>
                            </div>
                        </div>
                    </div>
                `;
            } else {
                this.dom.statsDetails.innerHTML = `<span style="color:var(--color-danger, #fb7185);"><i class="fa fa-exclamation-circle"></i> Supervisor connection offline.</span>`;
            }
        }
    }

    displayServices() {
        if (!this.dom.servicesList) return;
        this.dom.servicesList.innerHTML = '';

        if (!this.statusData.services || this.statusData.services.length === 0) {
            this.dom.servicesList.innerHTML = `
                <tr>
                    <td colspan="5" style="padding:40px; text-align:center; color:var(--color-text-muted, #94a3b8);">
                        No supervised services configured.
                    </td>
                </tr>
            `;
            return;
        }

        this.statusData.services.forEach(svc => {
            const row = document.createElement('tr');

            let dotClass = 'openmfe-dot--inactive';
            let stateText = 'Stopped';
            if (svc.running) {
                dotClass = 'openmfe-dot--active';
                stateText = 'Running';
            } else if (svc.one_shot && svc.completed) {
                dotClass = 'openmfe-dot--active';
                stateText = 'Completed';
            } else if (svc.one_shot && svc.failed) {
                dotClass = 'openmfe-dot--inactive';
                stateText = 'Failed';
            }

            const pidVal = svc.running && svc.pid ? svc.pid : '-';
            const depBadges = svc.deps && svc.deps.length > 0
                ? svc.deps.map(d => `<span class="openmfe-badge openmfe-badge--info">${this.escapeHTML(d)}</span>`).join('')
                : '<span style="color:var(--color-text-muted, #94a3b8); font-size:0.8rem;">None</span>';

            const isExternal = svc.name === "timescale-db" || svc.name === "rag-mcp";
            let actionHTML = '';
            if (isExternal) {
                if (svc.name === "timescale-db" && !svc.running) {
                    actionHTML = `<button class="openmfe-btn openmfe-btn--sm openmfe-btn--primary mfe-start-db-btn">
                            <i class="fa fa-play"></i> Start DB
                        </button>`;
                } else {
                    actionHTML = `<span style="color:var(--color-text-muted, #94a3b8); font-size:0.8125rem; font-style:italic;">External</span>`;
                }
            } else {
                actionHTML = `<button class="openmfe-btn openmfe-btn--sm openmfe-btn--outline mfe-restart-svc-btn" data-name="${this.escapeAttribute(svc.name)}">
                        <i class="fa fa-refresh"></i> Restart
                    </button>`;
            }

            row.innerHTML = `
                <td style="font-family:var(--font-mono, monospace); font-weight:700; color:var(--color-text-primary, #f8fafc);">${this.escapeHTML(svc.name)}</td>
                <td>
                    <span style="display:flex; align-items:center; gap:8px;">
                        <span class="openmfe-dot ${dotClass}"></span>
                        <span>${stateText}</span>
                    </span>
                </td>
                <td style="font-family:var(--font-mono, monospace);">${pidVal}</td>
                <td>${depBadges}</td>
                <td style="text-align:right;">${actionHTML}</td>
            `;

            // Bind click event handlers
            if (isExternal) {
                if (svc.name === "timescale-db" && !svc.running) {
                    row.querySelector('.mfe-start-db-btn')?.addEventListener('click', () => {
                        this.startPostgres();
                    });
                }
            } else {
                row.querySelector('.mfe-restart-svc-btn')?.addEventListener('click', (e) => {
                    const name = e.currentTarget.dataset.name;
                    this.restartService(name);
                });
            }

            this.dom.servicesList.appendChild(row);
        });
    }

    async startPostgres() {
        if (!confirm("Are you sure you want to attempt starting the Postgres database?")) return;
        try {
            let url = `${this.baseUrl}/api/v1/postgres/start`;
            let res = await fetch(url, { method: 'POST' }).catch(() => null);
            if ((!res || !res.ok) && typeof window !== 'undefined' && this.baseUrl !== window.location.origin) {
                res = await fetch(`${window.location.origin}/api/v1/postgres/start`, { method: 'POST' }).catch(() => null);
            }
            if (res && res.ok) {
                alert("Postgres start process initiated in the background.");
                await this.loadStatus();
            } else {
                const text = res ? await res.text() : 'Network error';
                alert(`Error launching database: ${text}`);
            }
        } catch (err) {
            alert(`Failed to trigger database launch: ${err.message}`);
        }
    }

    async restartService(name) {
        if (!confirm(`Are you sure you want to restart service '${name}'?`)) return;
        try {
            let url = `${this.baseUrl}/api/v1/watchdog/restart`;
            let res = await fetch(url, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ name })
            }).catch(() => null);
            if ((!res || !res.ok) && typeof window !== 'undefined' && this.baseUrl !== window.location.origin) {
                res = await fetch(`${window.location.origin}/api/v1/watchdog/restart`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ name })
                }).catch(() => null);
            }
            if (res && res.ok) {
                alert(`Restart triggered for service '${name}'.`);
                await this.loadStatus();
            } else {
                const text = res ? await res.text() : 'Network error';
                alert(`Error restarting service: ${text}`);
            }
        } catch (err) {
            alert(`Failed to restart service: ${err.message}`);
        }
    }

    async restartAll() {
        if (!confirm("Are you sure you want to restart ALL managed ecosystem services? This will cleanly shut down and rebuild everything sequentially!")) return;
        try {
            let url = `${this.baseUrl}/api/v1/watchdog/restart_all`;
            let res = await fetch(url, { method: 'POST' }).catch(() => null);
            if ((!res || !res.ok) && typeof window !== 'undefined' && this.baseUrl !== window.location.origin) {
                res = await fetch(`${window.location.origin}/api/v1/watchdog/restart_all`, { method: 'POST' }).catch(() => null);
            }
            if (res && res.ok) {
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

// =============================================================================
// Registration Ritual
// =============================================================================

const registerMFE = (typeof window !== 'undefined' && window.BastienMFEElement && window.BastienMFEElement.register)
    ? window.BastienMFEElement.register
    : (tag, cls) => {
        if (typeof customElements !== 'undefined' && !customElements.get(tag)) {
            try { customElements.define(tag, cls); }
            catch (e) { class Alias extends cls {} customElements.define(tag, Alias); }
        }
    };

registerMFE('watchdog-agent-mfe', WatchdogAgentMFE);

})();
