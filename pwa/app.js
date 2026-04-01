// BW Monitor PWA - Main Application
// Depends on services.js (KubeConfig, KubernetesClient, MonitorService, ipColor, flagEmoji, formatTime, formatDateTime)

'use strict';

// ── State ────────────────────────────────────────────────
const state = {
  connected: false,
  connecting: false,
  connectError: null,
  kubeConfigYAML: null,
  kubeConfigFileName: null,
  namespace: 'bunkerweb',
  maxEntries: 5000,
  contexts: [],        // [{name, cluster, user, namespace}]
  selectedContext: '',  // chosen context name (empty = current-context)
  reports: [],
  bans: [],
  totalReports: 0,
  currentTab: 'reports',
  currentView: 'list',
  detailItem: null,
  detailType: null,
  paused: false,
  live: false,
  filter: { ip: '', country: '', server: '', dateFrom: '', dateTo: '' },
  showFilter: false,
  showMenu: false,
  showExcludes: false,
  excludedIPs: new Set(),
  dnsCache: {},
  lastError: null,
  service: null,
  pollTimer: null,
};

// ── Helpers ──────────────────────────────────────────────
function esc(s) {
  if (!s) return '';
  const d = document.createElement('div');
  d.textContent = String(s);
  return d.innerHTML;
}

function filterActive() {
  const f = state.filter;
  return f.ip || f.country || f.server || f.dateFrom || f.dateTo;
}

function filterSummary() {
  const f = state.filter;
  const p = [];
  if (f.ip) p.push('IP:' + f.ip);
  if (f.country) p.push('CC:' + f.country);
  if (f.server) p.push('Server:' + f.server);
  if (f.dateFrom) p.push('From:' + f.dateFrom);
  if (f.dateTo) p.push('To:' + f.dateTo);
  return p.join(' / ');
}

function matchesReport(r) {
  const f = state.filter;
  if (f.ip && !(r.ip || '').includes(f.ip)) return false;
  if (f.country) {
    const codes = f.country.split(',').map(c => c.trim().toUpperCase());
    if (!codes.includes((r.country || '').toUpperCase())) return false;
  }
  if (f.server && !(r.server_name || '').toLowerCase().includes(f.server.toLowerCase())) return false;
  if (f.dateFrom) {
    const from = new Date(f.dateFrom).getTime() / 1000;
    if (r.date < from) return false;
  }
  if (f.dateTo) {
    const to = new Date(f.dateTo).getTime() / 1000;
    if (r.date > to) return false;
  }
  return true;
}

function matchesBan(b) {
  const f = state.filter;
  const d = b.data || {};
  if (f.ip && !(b.ip || '').includes(f.ip)) return false;
  if (f.country) {
    const codes = f.country.split(',').map(c => c.trim().toUpperCase());
    if (!codes.includes((d.country || '').toUpperCase())) return false;
  }
  if (f.server && !(d.service || '').toLowerCase().includes(f.server.toLowerCase())) return false;
  if (f.dateFrom) {
    const from = new Date(f.dateFrom).getTime() / 1000;
    if ((d.date || 0) < from) return false;
  }
  if (f.dateTo) {
    const to = new Date(f.dateTo).getTime() / 1000;
    if ((d.date || 0) > to) return false;
  }
  return true;
}

function getFilteredReports() {
  return state.reports.filter(r =>
    !state.excludedIPs.has(r.ip) && (!filterActive() || matchesReport(r))
  );
}

function getFilteredBans() {
  return state.bans.filter(b =>
    !state.excludedIPs.has(b.ip) && (!filterActive() || matchesBan(b))
  );
}

function relativeTime(unixTs) {
  const diff = Math.floor(Date.now() / 1000 - unixTs);
  if (diff < 60) return 'just now';
  if (diff < 3600) return Math.floor(diff / 60) + 'm ago';
  if (diff < 86400) {
    const h = Math.floor(diff / 3600);
    const m = Math.floor((diff % 3600) / 60);
    return h + 'h ' + m + 'm ago';
  }
  return Math.floor(diff / 86400) + 'd ago';
}

function expiresIn(ttlSec) {
  if (ttlSec <= 0) return 'Permanent';
  const h = Math.floor(ttlSec / 3600);
  const m = Math.floor((ttlSec % 3600) / 60);
  if (h > 0) return h + 'h ' + m + 'm';
  return m + 'm';
}

let countryNames;
try { countryNames = new Intl.DisplayNames(['en'], { type: 'region' }); } catch (e) { countryNames = null; }

function countryName(code) {
  if (!code) return '';
  try { return countryNames ? countryNames.of(code.toUpperCase()) : code; } catch { return code; }
}

function saveState() {
  try {
    localStorage.setItem('bwm_kubeconfig', state.kubeConfigYAML || '');
    localStorage.setItem('bwm_kubefile', state.kubeConfigFileName || '');
    localStorage.setItem('bwm_namespace', state.namespace);
    localStorage.setItem('bwm_excludes', JSON.stringify([...state.excludedIPs]));
    localStorage.setItem('bwm_context', state.selectedContext);
  } catch {}
}

function restoreState() {
  try {
    const kc = localStorage.getItem('bwm_kubeconfig');
    if (kc) { state.kubeConfigYAML = kc; state.kubeConfigFileName = localStorage.getItem('bwm_kubefile'); }
    const ns = localStorage.getItem('bwm_namespace');
    if (ns) state.namespace = ns;
    const ctx = localStorage.getItem('bwm_context');
    if (ctx) state.selectedContext = ctx;
    // Re-parse contexts from saved kubeconfig
    if (state.kubeConfigYAML) {
      try {
        const info = KubeConfig.listContexts(state.kubeConfigYAML);
        state.contexts = info.contexts;
        if (!state.selectedContext) state.selectedContext = info.currentContext;
      } catch {}
    }
    const ex = localStorage.getItem('bwm_excludes');
    if (ex) state.excludedIPs = new Set(JSON.parse(ex));
  } catch {}
}

// ── DNS Lookup ───────────────────────────────────────────
function lookupDNS(ip) {
  if (state.dnsCache[ip]) return;
  state.dnsCache[ip] = ['(looking up...)'];
  render();
  const parts = ip.split('.').reverse().join('.');
  fetch('https://dns.google/resolve?name=' + parts + '.in-addr.arpa&type=PTR')
    .then(r => r.json())
    .then(data => {
      if (data.Answer && data.Answer.length) {
        state.dnsCache[ip] = data.Answer.map(a => a.data.replace(/\.$/, ''));
      } else {
        state.dnsCache[ip] = ['(no rDNS)'];
      }
      render();
    })
    .catch(() => { state.dnsCache[ip] = ['(lookup failed)']; render(); });
}

// ── Connection ───────────────────────────────────────────
async function doConnect() {
  if (!state.kubeConfigYAML) { state.connectError = 'No kubeconfig loaded'; render(); return; }
  state.connecting = true;
  state.connectError = null;
  render();
  try {
    const config = KubeConfig.parse(state.kubeConfigYAML, state.selectedContext || undefined);
    const k8s = new KubernetesClient(config);
    const svc = new MonitorService(k8s, state.namespace);
    const pod = await svc.connect();
    state.service = svc;
    state.connected = true;
    state.connecting = false;
    state.live = true;
    saveState();
    render();
    const result = await svc.loadInitial(state.maxEntries);
    state.reports = result.reports;
    state.totalReports = result.total;
    render();
    startPolling();
  } catch (e) {
    state.connecting = false;
    state.connectError = e.message || String(e);
    render();
  }
}

function doDisconnect() {
  stopPolling();
  state.connected = false;
  state.live = false;
  state.service = null;
  state.reports = [];
  state.bans = [];
  state.totalReports = 0;
  state.showMenu = false;
  state.currentView = 'list';
  render();
}

function startPolling() {
  stopPolling();
  state.pollTimer = setInterval(async () => {
    if (state.paused || !state.service) return;
    try {
      const newReports = await state.service.pollNew();
      if (newReports && newReports.length) {
        state.reports = newReports.concat(state.reports);
        state.totalReports += newReports.length;
        state.lastError = null;
        render();
      }
    } catch (e) {
      state.lastError = e.message || String(e);
      render();
    }
  }, 3000);
}

function stopPolling() {
  if (state.pollTimer) { clearInterval(state.pollTimer); state.pollTimer = null; }
}

async function loadBans() {
  if (!state.service) return;
  try {
    state.bans = await state.service.loadBans();
    state.lastError = null;
  } catch (e) {
    state.lastError = 'Bans: ' + (e.message || String(e));
  }
  render();
}

// ── Render: Connection Screen ────────────────────────────
function renderConnectionScreen() {
  const hasFile = !!state.kubeConfigYAML;
  return `
    <div class="connect-screen">
      <div class="connect-logo">🛡️</div>
      <div class="connect-title">BW Monitor</div>
      <div class="connect-subtitle">BunkerWeb Security Monitor</div>
      <div class="connect-form">
        <button class="btn btn-file ${hasFile ? 'loaded' : ''}" onclick="document.getElementById('fileInput').click()">
          ${hasFile ? '✅ ' + esc(state.kubeConfigFileName) : '📄 Import Kubeconfig File'}
        </button>
        <input type="file" id="fileInput" style="display:none" onchange="handleFileImport(this)">
        ${state.contexts.length > 1 ? `
        <div class="form-group">
          <label>Context</label>
          <select id="ctxSelect" onchange="handleContextChange(this.value)" style="background:var(--surface);border:1px solid var(--border);border-radius:8px;padding:10px 12px;color:var(--text);font-size:14px;outline:none;appearance:auto;">
            ${state.contexts.map(c => `<option value="${esc(c.name)}" ${c.name === state.selectedContext ? 'selected' : ''}>${esc(c.name)}${c.namespace ? ' (' + esc(c.namespace) + ')' : ''}</option>`).join('')}
          </select>
        </div>` : ''}
        <div class="form-group">
          <label>Namespace</label>
          <input type="text" value="${esc(state.namespace)}" id="nsInput" placeholder="bunkerweb">
        </div>
        <button class="btn btn-primary" onclick="handleConnect()" ${!hasFile || state.connecting ? 'disabled' : ''}>
          ${state.connecting ? '⏳ Connecting...' : '⚡ Connect'}
        </button>
        ${state.connectError ? '<div class="connect-error">⚠️ ' + esc(state.connectError) + '</div>' : ''}
      </div>
    </div>`;
}

// ── Render: Main View ────────────────────────────────────
function renderMainView() {
  const isReports = state.currentTab === 'reports';
  const contextLabel = isReports ? 'Live View' : 'Active Bans';
  const items = isReports ? getFilteredReports() : getFilteredBans();
  const total = isReports ? state.totalReports : state.bans.length;

  let listHTML;
  if (items.length === 0) {
    const icon = isReports ? '🛡️' : '✋';
    const msg = filterActive()
      ? 'No ' + (isReports ? 'reports' : 'bans') + ' match the current filter'
      : isReports ? 'Waiting for blocked requests...' : 'No active bans found';
    listHTML = `<div class="list-empty"><div class="list-empty-icon">${icon}</div><div>${msg}</div></div>`;
  } else if (isReports) {
    listHTML = items.map((r, i) => renderReportRow(r, i)).join('');
  } else {
    listHTML = items.map((b, i) => renderBanRow(b, i)).join('');
  }

  const pauseBadge = isReports
    ? (state.paused
      ? '<span class="badge badge-paused">PAUSED</span>'
      : (state.live ? '<span class="badge badge-live">LIVE</span>' : ''))
    : '';

  let statusParts = [`${items.length}/${total}`];
  if (filterActive()) statusParts.push(`<span class="status-filter">${esc(filterSummary())}</span>`);
  if (state.excludedIPs.size) statusParts.push(`${state.excludedIPs.size} excluded`);
  if (state.lastError) statusParts.push(`<span class="status-error">⚠ ${esc(state.lastError)}</span>`);

  return `
    <div class="app-container">
      <div class="title-bar">
        <div class="title-bar-left">
          <h1>BW Monitor</h1>
          ${pauseBadge}
        </div>
        <div class="title-bar-context">${contextLabel}</div>
        <div class="title-bar-actions">
          ${!isReports ? '<button onclick="loadBans()" title="Refresh">↻</button>' : ''}
          <button onclick="toggleMenu()">⋯</button>
        </div>
      </div>
      <div class="list-container">${listHTML}</div>
      <div class="status-bar">${statusParts.join(' &nbsp;|&nbsp; ')}</div>
      <div class="tab-bar">
        <button class="tab-btn ${state.currentTab === 'reports' ? 'active' : ''}" onclick="switchTab('reports')">
          <span class="tab-btn-icon">🛡️</span>Reports
        </button>
        <button class="tab-btn ${state.currentTab === 'bans' ? 'active' : ''}" onclick="switchTab('bans')">
          <span class="tab-btn-icon">✋</span>Bans
        </button>
      </div>
      ${state.showMenu ? renderMenu() : ''}
      ${state.showFilter ? renderFilterModal() : ''}
      ${state.showExcludes ? renderExcludesModal() : ''}
    </div>`;
}

function renderReportRow(r, idx) {
  const color = ipColor(r.ip);
  const mc = 'method-' + (r.method || 'GET').toUpperCase();
  const statusColor = (r.status >= 400) ? 'var(--red)' : 'var(--text-dim)';
  return `
    <div class="report-row" onclick="openReportDetail(${idx})">
      <div class="report-row-top">
        <span class="report-ip" style="color:${color}">${esc(r.ip)}</span>
        <span class="report-country">${flagEmoji(r.country)} ${esc(r.country)}</span>
        <span class="report-time">${formatTime(r.date)}</span>
      </div>
      <div class="report-row-mid">
        <span class="method-badge ${mc}">${esc(r.method)}</span>
        <span class="report-status" style="color:${statusColor}">${r.status}</span>
        <span class="report-reason">${esc(r.reason)}</span>
      </div>
      <div class="report-row-bot">
        <span class="report-server">${esc(r.server_name)}</span>
        <span class="report-url">${esc(r.url)}</span>
      </div>
    </div>`;
}

function renderBanRow(b, idx) {
  const d = b.data || {};
  const color = ipColor(b.ip);
  const events = d.reason_data || [];
  const expiry = d.permanent ? '<span class="ban-permanent">PERMANENT</span>' : '<span class="ban-expires">' + expiresIn(b.ttl) + '</span>';
  return `
    <div class="ban-row" onclick="openBanDetail(${idx})">
      <div class="ban-row-top">
        <span class="report-ip" style="color:${color}">${esc(b.ip)}</span>
        <span class="report-country">${flagEmoji(d.country)} ${esc(d.country)}</span>
        <span class="ban-events-badge">${events.length}</span>
      </div>
      <div class="ban-row-mid">
        <span class="report-reason">${esc(d.reason)}</span>
        ${expiry}
      </div>
      <div class="ban-row-bot">
        <span class="report-server">${esc(d.service)}</span>
        <span class="ban-time">${relativeTime(d.date)}</span>
      </div>
    </div>`;
}

// ── Render: Detail View ──────────────────────────────────
function renderDetailView() {
  if (state.detailType === 'report') return renderReportDetail(state.detailItem);
  return renderBanDetail(state.detailItem);
}

function renderReportDetail(r) {
  const color = ipColor(r.ip);
  const dns = state.dnsCache[r.ip];
  const dnsText = dns ? dns.join(', ') : '';

  let badBehaviorHTML = '';
  if (r.data && Array.isArray(r.data) && r.data.length) {
    badBehaviorHTML = `
      <div class="detail-section">
        <div class="detail-section-title">Bad Behavior History</div>
        ${r.data.map((d, i) => `
          <div class="event-item">
            <div class="event-header">
              <span class="event-num">Event ${i + 1}</span>
              <span class="event-time">${formatDateTime(d.date)}</span>
            </div>
            <div class="event-url">${esc(d.url)}</div>
            <div class="detail-field"><span class="detail-label">Method</span><span class="detail-value">${esc(d.method)}</span></div>
            <div class="detail-field"><span class="detail-label">Status</span><span class="detail-value">${esc(d.status)}</span></div>
            <div class="detail-field"><span class="detail-label">Ban Time</span><span class="detail-value">${d.ban_time}s</span></div>
            <div class="detail-field"><span class="detail-label">Ban Scope</span><span class="detail-value">${esc(d.ban_scope)}</span></div>
            <div class="detail-field"><span class="detail-label">Threshold</span><span class="detail-value">${d.threshold}</span></div>
            <div class="detail-field"><span class="detail-label">Count Time</span><span class="detail-value">${d.count_time}s</span></div>
          </div>
        `).join('')}
      </div>`;
  }

  return `
    <div class="detail-view">
      <div class="detail-header">
        <button onclick="closeDetail()">←</button>
        <h2>Block Detail</h2>
      </div>
      <div class="detail-content">
        <div class="detail-section">
          <div class="detail-section-title">Request</div>
          <div class="detail-field"><span class="detail-label">Request ID</span><span class="detail-value mono">${esc(r.id)}</span></div>
          <div class="detail-field"><span class="detail-label">Date/Time</span><span class="detail-value">${formatDateTime(r.date)}</span></div>
        </div>
        <div class="detail-section">
          <div class="detail-section-title">Source</div>
          <div class="detail-field"><span class="detail-label">IP Address</span><span class="detail-value mono" style="color:${color}">${esc(r.ip)}</span></div>
          ${dnsText ? '<div class="detail-field"><span class="detail-label">rDNS</span><span class="detail-value">' + esc(dnsText) + '</span></div>' : ''}
          <div class="detail-field"><span class="detail-label">Country</span><span class="detail-value">${flagEmoji(r.country)} ${esc(countryName(r.country))}</span></div>
        </div>
        <div class="detail-section">
          <div class="detail-section-title">Request Details</div>
          <div class="detail-field"><span class="detail-label">Method</span><span class="detail-value">${esc(r.method)}</span></div>
          <div class="detail-field"><span class="detail-label">URL</span><span class="detail-value mono">${esc(r.url)}</span></div>
          <div class="detail-field"><span class="detail-label">Status</span><span class="detail-value">${r.status}</span></div>
          <div class="detail-field"><span class="detail-label">Reason</span><span class="detail-value" style="color:var(--orange)">${esc(r.reason)}</span></div>
          <div class="detail-field"><span class="detail-label">Server</span><span class="detail-value" style="color:var(--teal)">${esc(r.server_name)}</span></div>
          <div class="detail-field"><span class="detail-label">Security Mode</span><span class="detail-value">${esc(r.security_mode)}</span></div>
        </div>
        <div class="detail-section">
          <div class="detail-section-title">User Agent</div>
          <div style="padding:6px 0;font-family:'SF Mono',monospace;font-size:12px;color:var(--text-dim);word-break:break-all;user-select:text;-webkit-user-select:text;">
            ${esc(r.user_agent || '(none)')}
          </div>
        </div>
        ${badBehaviorHTML}
      </div>
    </div>`;
}

function renderBanDetail(b) {
  const d = b.data || {};
  const color = ipColor(b.ip);
  const dns = state.dnsCache[b.ip];
  const dnsText = dns ? dns.join(', ') : '';
  const events = d.reason_data || [];

  let eventsHTML = '';
  if (events.length) {
    eventsHTML = `
      <div class="detail-section">
        <div class="detail-section-title">Events (${events.length} requests led to ban)</div>
        ${events.map((e, i) => `
          <div class="event-item">
            <div class="event-header">
              <span class="event-num">[${i + 1}]</span>
              <span class="event-time">${formatTime(e.date)}</span>
              <span class="method-badge method-${(e.method || 'GET').toUpperCase()}">${esc(e.method)}</span>
              <span style="color:var(--text-dim)">→ ${esc(e.status)}</span>
            </div>
            <div class="event-url">${esc(e.url)}</div>
          </div>
        `).join('')}
      </div>`;
    if (events[0]) {
      eventsHTML += `
        <div class="detail-section">
          <div class="detail-section-title">Summary</div>
          <div style="padding:8px 0;font-size:13px;color:var(--text-dim)">
            Ban triggered after ${events.length} requests in ${events[0].count_time}s (threshold: ${events[0].threshold})
          </div>
        </div>`;
    }
  }

  return `
    <div class="detail-view">
      <div class="detail-header">
        <button onclick="closeDetail()">←</button>
        <h2>Ban Detail</h2>
      </div>
      <div class="detail-content">
        <div class="detail-section">
          <div class="detail-section-title">Ban Information</div>
          <div class="detail-field"><span class="detail-label">IP Address</span><span class="detail-value mono" style="color:${color}">${esc(b.ip)}</span></div>
          ${dnsText ? '<div class="detail-field"><span class="detail-label">rDNS</span><span class="detail-value">' + esc(dnsText) + '</span></div>' : ''}
          <div class="detail-field"><span class="detail-label">Country</span><span class="detail-value">${flagEmoji(d.country)} ${esc(countryName(d.country))}</span></div>
          <div class="detail-field"><span class="detail-label">Service</span><span class="detail-value" style="color:var(--teal)">${esc(d.service)}</span></div>
          <div class="detail-field"><span class="detail-label">Reason</span><span class="detail-value" style="color:var(--orange)">${esc(d.reason)}</span></div>
          <div class="detail-field"><span class="detail-label">Ban Scope</span><span class="detail-value">${esc(d.ban_scope)}</span></div>
        </div>
        <div class="detail-section">
          <div class="detail-section-title">Timing</div>
          <div class="detail-field"><span class="detail-label">Banned At</span><span class="detail-value">${formatDateTime(d.date)}</span></div>
          ${d.permanent
            ? '<div class="detail-field"><span class="detail-label">Expires</span><span class="detail-value" style="color:var(--red)">Never (permanent)</span></div>'
            : '<div class="detail-field"><span class="detail-label">Expires In</span><span class="detail-value">' + expiresIn(b.ttl) + '</span></div>'}
        </div>
        ${eventsHTML}
      </div>
    </div>`;
}

// ── Render: Modals ───────────────────────────────────────
function renderMenu() {
  const isReports = state.currentTab === 'reports';
  return `
    <div class="menu-overlay" onclick="toggleMenu()"></div>
    <div class="menu-dropdown">
      ${isReports ? `<button class="menu-item" onclick="togglePause()">${state.paused ? '▶️ Resume' : '⏸️ Pause'}</button>` : ''}
      <button class="menu-item" onclick="openFilter()">🔍 Filter</button>
      ${filterActive() ? '<button class="menu-item" onclick="clearFilter()">✕ Clear Filter</button>' : ''}
      ${state.excludedIPs.size ? '<button class="menu-item" onclick="openExcludes()">👁️ Excluded IPs (' + state.excludedIPs.size + ')</button>' : ''}
      <div class="menu-divider"></div>
      <button class="menu-item danger" onclick="doDisconnect()">⚡ Disconnect</button>
    </div>`;
}

function renderFilterModal() {
  const f = state.filter;
  return `
    <div class="modal-overlay" onclick="closeFilter()">
      <div class="modal-sheet" onclick="event.stopPropagation()">
        <div class="modal-handle"></div>
        <div class="modal-title">Filter</div>
        <div class="form-group">
          <label>IP Address (substring)</label>
          <input type="text" id="fIP" value="${esc(f.ip)}" placeholder="e.g. 192.168" autocomplete="off" autocapitalize="off">
        </div>
        <div class="form-group">
          <label>Country Code (comma-separated)</label>
          <input type="text" id="fCountry" value="${esc(f.country)}" placeholder="e.g. GB,US" autocomplete="off" style="text-transform:uppercase">
        </div>
        <div class="form-group">
          <label>Server Name (substring)</label>
          <input type="text" id="fServer" value="${esc(f.server)}" placeholder="e.g. example.com" autocomplete="off" autocapitalize="off">
        </div>
        <div class="form-group">
          <label>From</label>
          <input type="datetime-local" id="fFrom" value="${esc(f.dateFrom)}">
        </div>
        <div class="form-group">
          <label>To</label>
          <input type="datetime-local" id="fTo" value="${esc(f.dateTo)}">
        </div>
        <div class="modal-actions">
          <button class="btn btn-secondary" onclick="closeFilter()">Cancel</button>
          ${filterActive() ? '<button class="btn btn-danger" onclick="clearFilter()">Clear</button>' : ''}
          <button class="btn btn-primary" onclick="applyFilter()">Apply</button>
        </div>
      </div>
    </div>`;
}

function renderExcludesModal() {
  const ips = [...state.excludedIPs].sort();
  return `
    <div class="modal-overlay" onclick="closeExcludes()">
      <div class="modal-sheet" onclick="event.stopPropagation()">
        <div class="modal-handle"></div>
        <div class="modal-title">Excluded IPs</div>
        ${ips.length === 0 ? '<div style="color:var(--text-dim);padding:16px 0">No excluded IPs</div>' : ''}
        ${ips.map(ip => `
          <div class="exclude-item">
            <span class="exclude-ip" style="color:${ipColor(ip)}">${esc(ip)}</span>
            <button class="exclude-remove" onclick="removeExclude('${esc(ip)}')">✕</button>
          </div>
        `).join('')}
        <div class="modal-actions">
          <button class="btn btn-secondary" onclick="closeExcludes()">Done</button>
        </div>
      </div>
    </div>`;
}

// ── Event Handlers ───────────────────────────────────────
window.handleFileImport = function(input) {
  const file = input.files[0];
  if (!file) return;
  const reader = new FileReader();
  reader.onload = () => {
    state.kubeConfigYAML = reader.result;
    state.kubeConfigFileName = file.name;
    // Extract available contexts
    try {
      const info = KubeConfig.listContexts(reader.result);
      state.contexts = info.contexts;
      state.selectedContext = info.currentContext;
    } catch (e) {
      state.contexts = [];
      state.selectedContext = '';
    }
    saveState();
    render();
  };
  reader.readAsText(file);
};

window.handleContextChange = function(contextName) {
  state.selectedContext = contextName;
  // Update namespace from the selected context if it has one
  const ctx = state.contexts.find(c => c.name === contextName);
  if (ctx && ctx.namespace) {
    state.namespace = ctx.namespace;
  }
  saveState();
  render();
};

window.handleConnect = function() {
  const nsInput = document.getElementById('nsInput');
  if (nsInput) state.namespace = nsInput.value.trim() || 'bunkerweb';
  doConnect();
};

window.switchTab = function(tab) {
  if (state.currentTab === tab) return;
  state.currentTab = tab;
  state.currentView = 'list';
  state.showMenu = false;
  if (tab === 'bans' && state.bans.length === 0) loadBans();
  render();
};

window.toggleMenu = function() {
  state.showMenu = !state.showMenu;
  render();
};

window.togglePause = function() {
  state.paused = !state.paused;
  state.showMenu = false;
  render();
};

window.openFilter = function() {
  state.showMenu = false;
  state.showFilter = true;
  render();
};

window.closeFilter = function() {
  state.showFilter = false;
  render();
};

window.applyFilter = function() {
  state.filter.ip = (document.getElementById('fIP')?.value || '').trim();
  state.filter.country = (document.getElementById('fCountry')?.value || '').trim();
  state.filter.server = (document.getElementById('fServer')?.value || '').trim();
  state.filter.dateFrom = document.getElementById('fFrom')?.value || '';
  state.filter.dateTo = document.getElementById('fTo')?.value || '';
  state.showFilter = false;
  render();
};

window.clearFilter = function() {
  state.filter = { ip: '', country: '', server: '', dateFrom: '', dateTo: '' };
  state.showFilter = false;
  state.showMenu = false;
  render();
};

window.openExcludes = function() {
  state.showMenu = false;
  state.showExcludes = true;
  render();
};

window.closeExcludes = function() {
  state.showExcludes = false;
  render();
};

window.removeExclude = function(ip) {
  state.excludedIPs.delete(ip);
  saveState();
  render();
};

window.openReportDetail = function(filteredIdx) {
  const items = getFilteredReports();
  if (filteredIdx < 0 || filteredIdx >= items.length) return;
  state.detailItem = items[filteredIdx];
  state.detailType = 'report';
  state.currentView = 'detail';
  lookupDNS(state.detailItem.ip);
  render();
};

window.openBanDetail = function(filteredIdx) {
  const items = getFilteredBans();
  if (filteredIdx < 0 || filteredIdx >= items.length) return;
  state.detailItem = items[filteredIdx];
  state.detailType = 'ban';
  state.currentView = 'detail';
  lookupDNS(state.detailItem.ip);
  render();
};

window.closeDetail = function() {
  state.currentView = 'list';
  state.detailItem = null;
  render();
};

window.loadBans = loadBans;

// Long-press / swipe to exclude (simplified: add exclude buttons contextually)
// For mobile, we add a touch-hold handler
let longPressTimer = null;
document.addEventListener('touchstart', e => {
  const row = e.target.closest('.report-row, .ban-row');
  if (!row) return;
  longPressTimer = setTimeout(() => {
    const idx = parseInt(row.getAttribute('onclick')?.match(/\d+/)?.[0]);
    if (isNaN(idx)) return;
    const items = state.currentTab === 'reports' ? getFilteredReports() : getFilteredBans();
    if (idx >= 0 && idx < items.length) {
      const ip = items[idx].ip;
      if (confirm('Exclude IP ' + ip + '?')) {
        state.excludedIPs.add(ip);
        saveState();
        render();
      }
    }
  }, 600);
});
document.addEventListener('touchend', () => { clearTimeout(longPressTimer); });
document.addEventListener('touchmove', () => { clearTimeout(longPressTimer); });

// ── Main Render ──────────────────────────────────────────
function render() {
  const app = document.getElementById('app');
  if (!state.connected) {
    app.innerHTML = renderConnectionScreen();
  } else if (state.currentView === 'detail') {
    app.innerHTML = renderDetailView();
  } else {
    app.innerHTML = renderMainView();
  }
}

// ── Init ─────────────────────────────────────────────────
restoreState();
render();

if ('serviceWorker' in navigator) {
  navigator.serviceWorker.register('sw.js').catch(() => {});
}
