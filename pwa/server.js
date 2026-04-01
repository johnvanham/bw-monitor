#!/usr/bin/env node
//
// BW Monitor PWA Server
//
// Serves the PWA static files and proxies Kubernetes API requests to avoid
// browser CORS restrictions. Reads your kubeconfig automatically.
//
// Usage:
//   node server.js [options]
//
// Options:
//   --port PORT          Listen port (default: 3000)
//   --kubeconfig PATH    Kubeconfig file (default: $KUBECONFIG which may be colon-separated, or ~/.kube/config)
//   --context NAME       Kubeconfig context (default: current-context)
//   --address ADDR       Listen address (default: 0.0.0.0)
//

'use strict';

const http = require('http');
const https = require('https');
const fs = require('fs');
const path = require('path');
const url = require('url');
const crypto = require('crypto');

// ── Parse CLI args ──────────────────────────────────────────
const args = process.argv.slice(2);
function getArg(name, def) {
  const idx = args.indexOf('--' + name);
  return idx >= 0 && idx + 1 < args.length ? args[idx + 1] : def;
}

const PORT = parseInt(getArg('port', '3000'), 10);
const ADDRESS = getArg('address', '0.0.0.0');
const KUBECONFIG_PATHS = (function() {
  const explicit = getArg('kubeconfig', '');
  if (explicit) return [explicit];
  const env = process.env.KUBECONFIG || '';
  if (env) {
    const sep = process.platform === 'win32' ? ';' : ':';
    return env.split(sep).filter(Boolean);
  }
  return [path.join(process.env.HOME || process.env.USERPROFILE || '.', '.kube', 'config')];
})();
const CONTEXT_NAME = getArg('context', '');

// ── Minimal YAML parser (kubeconfig subset) ─────────────────
function parseYAML(text) {
  const lines = text.replace(/\r\n/g, '\n').split('\n');
  let idx = 0;

  function skipBlank() {
    while (idx < lines.length && (/^\s*$/.test(lines[idx]) || /^\s*#/.test(lines[idx]))) idx++;
  }
  function indentOf(line) { const m = line.match(/^(\s*)/); return m ? m[1].length : 0; }
  function splitKV(line) {
    const t = line.trim();
    for (let i = 0; i < t.length; i++) {
      if (t[i] === ':' && (i === t.length - 1 || t[i + 1] === ' ')) {
        return { key: t.slice(0, i).trim(), value: t.slice(i + 1).trim() };
      }
    }
    return null;
  }
  function parseScalar(raw) {
    if (!raw || raw === '~' || raw === 'null') return null;
    if (raw === 'true') return true;
    if (raw === 'false') return false;
    if ((raw[0] === '"' || raw[0] === "'") && raw[0] === raw[raw.length - 1]) return raw.slice(1, -1);
    if (/^-?\d+$/.test(raw)) return parseInt(raw, 10);
    if (/^-?\d+\.\d+$/.test(raw)) return parseFloat(raw);
    return raw;
  }

  function parseNode(minIndent) {
    skipBlank();
    if (idx >= lines.length) return null;
    return lines[idx].trim().startsWith('- ') ? parseSeq(indentOf(lines[idx])) : parseMap(minIndent);
  }
  function parseMap(minIndent) {
    const obj = {};
    while (idx < lines.length) {
      skipBlank();
      if (idx >= lines.length) break;
      const line = lines[idx], indent = indentOf(line), trimmed = line.trim();
      if (indent < minIndent || trimmed.startsWith('- ')) break;
      const kv = splitKV(trimmed);
      if (!kv) { idx++; continue; }
      idx++;
      let val = kv.value;
      if (val && val[0] !== '"' && val[0] !== "'") { const ci = val.indexOf(' #'); if (ci >= 0) val = val.slice(0, ci).trim(); }
      if (!val || val === '~' || val === 'null') {
        skipBlank();
        if (idx < lines.length) {
          const ni = indentOf(lines[idx]);
          if (ni > indent) obj[kv.key] = parseNode(ni);
          else if (ni >= indent && lines[idx].trim().startsWith('- ')) obj[kv.key] = parseSeq(ni);
          else obj[kv.key] = null;
        } else obj[kv.key] = null;
      } else obj[kv.key] = parseScalar(val);
    }
    return obj;
  }
  function parseSeq(seqIndent) {
    const arr = [];
    while (idx < lines.length) {
      skipBlank();
      if (idx >= lines.length) break;
      const line = lines[idx], indent = indentOf(line), trimmed = line.trim();
      if (indent < seqIndent || !trimmed.startsWith('- ')) break;
      const after = trimmed.slice(2).trim();
      const itemIndent = indent + 2;
      idx++;
      if (!after || after === '~') {
        skipBlank();
        arr.push(idx < lines.length && indentOf(lines[idx]) >= itemIndent ? parseNode(itemIndent) : null);
      } else {
        const kv = splitKV(after);
        if (kv) {
          const item = {};
          let val = kv.value;
          if (val && val[0] !== '"' && val[0] !== "'") { const ci = val.indexOf(' #'); if (ci >= 0) val = val.slice(0, ci).trim(); }
          if (!val || val === '~' || val === 'null') {
            skipBlank();
            if (idx < lines.length && indentOf(lines[idx]) > indent) item[kv.key] = parseNode(indentOf(lines[idx]));
            else item[kv.key] = null;
          } else item[kv.key] = parseScalar(val);
          while (idx < lines.length) {
            skipBlank(); if (idx >= lines.length) break;
            const cl = lines[idx], ci2 = indentOf(cl), ct = cl.trim();
            if (ci2 < itemIndent || ct.startsWith('- ')) break;
            const ckv = splitKV(ct); if (!ckv) { idx++; continue; } idx++;
            let cv = ckv.value;
            if (cv && cv[0] !== '"' && cv[0] !== "'") { const x = cv.indexOf(' #'); if (x >= 0) cv = cv.slice(0, x).trim(); }
            if (!cv || cv === '~' || cv === 'null') {
              skipBlank();
              if (idx < lines.length && indentOf(lines[idx]) > ci2) item[ckv.key] = parseNode(indentOf(lines[idx]));
              else item[ckv.key] = null;
            } else item[ckv.key] = parseScalar(cv);
          }
          arr.push(item);
        } else arr.push(parseScalar(after));
      }
    }
    return arr;
  }

  while (idx < lines.length && /^\s*---\s*$/.test(lines[idx])) idx++;
  return parseNode(0);
}

// ── Load kubeconfig ─────────────────────────────────────────
// Merges multiple kubeconfig files (kubectl-style): arrays are concatenated,
// first current-context wins.
function mergeKubeConfigs(filePaths) {
  const merged = { 'current-context': '', clusters: [], contexts: [], users: [] };
  for (const fp of filePaths) {
    let yaml;
    try { yaml = fs.readFileSync(fp, 'utf8'); } catch (e) {
      console.warn(`Warning: cannot read kubeconfig ${fp}: ${e.message}`);
      continue;
    }
    const doc = parseYAML(yaml);
    if (!doc) continue;
    if (!merged['current-context'] && doc['current-context']) merged['current-context'] = doc['current-context'];
    if (Array.isArray(doc.clusters)) merged.clusters.push(...doc.clusters);
    if (Array.isArray(doc.contexts)) merged.contexts.push(...doc.contexts);
    if (Array.isArray(doc.users)) merged.users.push(...doc.users);
  }
  return merged;
}

function loadKubeConfig(filePaths, contextName) {
  const doc = mergeKubeConfigs(filePaths);
  if (!doc.clusters.length && !doc.contexts.length) throw new Error('No valid kubeconfig data found in: ' + filePaths.join(', '));

  const ctxName = contextName || doc['current-context'] || '';
  const contexts = doc.contexts || [];
  const clusters = doc.clusters || [];
  const users = doc.users || [];

  let clusterName = '', userName = '', namespace = 'default';

  if (ctxName) {
    const ctx = contexts.find(c => c.name === ctxName);
    if (ctx && ctx.context) {
      clusterName = ctx.context.cluster || '';
      userName = ctx.context.user || '';
      namespace = ctx.context.namespace || 'default';
    }
  }
  if (!clusterName && clusters.length) clusterName = clusters[0].name || '';
  if (!userName && users.length) userName = users[0].name || '';

  const clusterEntry = clusters.find(c => c.name === clusterName);
  const cluster = clusterEntry ? clusterEntry.cluster || {} : {};
  const userEntry = users.find(u => u.name === userName);
  const user = userEntry ? userEntry.user || {} : {};

  const config = {
    server: (cluster.server || '').replace(/\/+$/, ''),
    caCert: cluster['certificate-authority-data']
      ? Buffer.from(cluster['certificate-authority-data'], 'base64')
      : (cluster['certificate-authority'] ? fs.readFileSync(cluster['certificate-authority']) : null),
    insecure: cluster['insecure-skip-tls-verify'] === true,
    token: user.token || null,
    username: user.username || null,
    password: user.password || null,
    clientCert: user['client-certificate-data']
      ? Buffer.from(user['client-certificate-data'], 'base64')
      : (user['client-certificate'] ? fs.readFileSync(user['client-certificate']) : null),
    clientKey: user['client-key-data']
      ? Buffer.from(user['client-key-data'], 'base64')
      : (user['client-key'] ? fs.readFileSync(user['client-key']) : null),
    namespace,
    contextName: ctxName,
    allContexts: contexts.map(c => ({
      name: c.name,
      cluster: (c.context || {}).cluster || '',
      namespace: (c.context || {}).namespace || '',
    })),
  };

  return config;
}

let kubeConfig;
try {
  kubeConfig = loadKubeConfig(KUBECONFIG_PATHS, CONTEXT_NAME);
  console.log(`Loaded kubeconfig from ${KUBECONFIG_PATHS.join(', ')}`);
  console.log(`  Context: ${kubeConfig.contextName || '(default)'}`);
  console.log(`  Server:  ${kubeConfig.server}`);
  console.log(`  Auth:    ${kubeConfig.token ? 'token' : kubeConfig.clientCert ? 'client-cert' : kubeConfig.username ? 'basic' : 'none'}`);
} catch (e) {
  console.error(`Failed to load kubeconfig: ${e.message}`);
  process.exit(1);
}

// ── TLS options for K8s API requests ────────────────────────
function tlsOptions() {
  const opts = {};
  if (kubeConfig.caCert) opts.ca = kubeConfig.caCert;
  if (kubeConfig.clientCert) opts.cert = kubeConfig.clientCert;
  if (kubeConfig.clientKey) opts.key = kubeConfig.clientKey;
  if (kubeConfig.insecure) opts.rejectUnauthorized = false;
  return opts;
}

function authHeaders() {
  const h = {};
  if (kubeConfig.token) h['Authorization'] = 'Bearer ' + kubeConfig.token;
  else if (kubeConfig.username && kubeConfig.password)
    h['Authorization'] = 'Basic ' + Buffer.from(kubeConfig.username + ':' + kubeConfig.password).toString('base64');
  return h;
}

// ── Static file serving ─────────────────────────────────────
const MIME = {
  '.html': 'text/html', '.css': 'text/css', '.js': 'application/javascript',
  '.json': 'application/json', '.png': 'image/png', '.svg': 'image/svg+xml',
  '.ico': 'image/x-icon', '.webmanifest': 'application/manifest+json',
};
const STATIC_DIR = __dirname;

function serveStatic(req, res) {
  let filePath = path.join(STATIC_DIR, req.url === '/' ? 'index.html' : req.url.split('?')[0]);
  filePath = path.normalize(filePath);
  if (!filePath.startsWith(STATIC_DIR)) { res.writeHead(403); res.end(); return; }

  fs.readFile(filePath, (err, data) => {
    if (err) { res.writeHead(404); res.end('Not found'); return; }
    const ext = path.extname(filePath);
    res.writeHead(200, { 'Content-Type': MIME[ext] || 'application/octet-stream' });
    res.end(data);
  });
}

// ── Proxy info endpoint ─────────────────────────────────────
function serveProxyInfo(res) {
  res.writeHead(200, { 'Content-Type': 'application/json' });
  res.end(JSON.stringify({
    proxy: true,
    server: kubeConfig.server,
    context: kubeConfig.contextName,
    namespace: kubeConfig.namespace,
    contexts: kubeConfig.allContexts,
  }));
}

// ── K8s API proxy (HTTP) ────────────────────────────────────
function proxyRequest(clientReq, clientRes) {
  const target = new URL(kubeConfig.server + clientReq.url);
  const isHttps = target.protocol === 'https:';
  const mod = isHttps ? https : http;

  const opts = {
    hostname: target.hostname,
    port: target.port || (isHttps ? 443 : 80),
    path: target.pathname + target.search,
    method: clientReq.method,
    headers: { ...clientReq.headers, ...authHeaders(), host: target.host },
    ...tlsOptions(),
  };
  delete opts.headers['origin'];
  delete opts.headers['referer'];

  const proxyReq = mod.request(opts, proxyRes => {
    // Add CORS headers
    const resHeaders = { ...proxyRes.headers };
    resHeaders['access-control-allow-origin'] = '*';
    resHeaders['access-control-allow-methods'] = 'GET, POST, PUT, DELETE, OPTIONS';
    resHeaders['access-control-allow-headers'] = '*';
    clientRes.writeHead(proxyRes.statusCode, resHeaders);
    proxyRes.pipe(clientRes);
  });

  proxyReq.on('error', err => {
    clientRes.writeHead(502, { 'Content-Type': 'text/plain' });
    clientRes.end('Proxy error: ' + err.message);
  });

  clientReq.pipe(proxyReq);
}

// ── K8s API proxy (WebSocket) ───────────────────────────────
function proxyWebSocket(clientReq, clientSocket, head) {
  const target = new URL(kubeConfig.server + clientReq.url);
  const isHttps = target.protocol === 'https:';

  // Build upgrade request headers
  const headers = { ...authHeaders() };
  // Forward relevant WebSocket headers
  for (const key of ['upgrade', 'connection', 'sec-websocket-key', 'sec-websocket-version', 'sec-websocket-protocol']) {
    if (clientReq.headers[key]) headers[key] = clientReq.headers[key];
  }
  headers['host'] = target.host;

  const opts = {
    hostname: target.hostname,
    port: target.port || (isHttps ? 443 : 80),
    path: target.pathname + target.search,
    method: 'GET',
    headers,
    ...tlsOptions(),
  };

  const mod = isHttps ? https : http;
  const proxyReq = mod.request(opts);

  proxyReq.on('upgrade', (proxyRes, proxySocket, proxyHead) => {
    // Send 101 back to client
    let response = 'HTTP/1.1 101 Switching Protocols\r\n';
    for (const [key, val] of Object.entries(proxyRes.headers)) {
      response += key + ': ' + val + '\r\n';
    }
    response += '\r\n';
    clientSocket.write(response);
    if (proxyHead.length) clientSocket.write(proxyHead);

    // Bidirectional pipe
    proxySocket.pipe(clientSocket);
    clientSocket.pipe(proxySocket);

    proxySocket.on('error', () => clientSocket.destroy());
    clientSocket.on('error', () => proxySocket.destroy());
    proxySocket.on('close', () => clientSocket.destroy());
    clientSocket.on('close', () => proxySocket.destroy());
  });

  proxyReq.on('error', err => {
    clientSocket.write('HTTP/1.1 502 Bad Gateway\r\n\r\n');
    clientSocket.destroy();
  });

  proxyReq.end();
}

// ── HTTP server ─────────────────────────────────────────────
const server = http.createServer((req, res) => {
  // CORS preflight
  if (req.method === 'OPTIONS') {
    res.writeHead(204, {
      'Access-Control-Allow-Origin': '*',
      'Access-Control-Allow-Methods': 'GET, POST, PUT, DELETE, OPTIONS',
      'Access-Control-Allow-Headers': '*',
    });
    res.end();
    return;
  }

  // Proxy info
  if (req.url === '/proxy-info') {
    serveProxyInfo(res);
    return;
  }

  // K8s API proxy
  if (req.url.startsWith('/api/')) {
    proxyRequest(req, res);
    return;
  }

  // Static files
  serveStatic(req, res);
});

// Handle WebSocket upgrade for exec
server.on('upgrade', (req, socket, head) => {
  if (req.url.startsWith('/api/')) {
    proxyWebSocket(req, socket, head);
  } else {
    socket.destroy();
  }
});

server.listen(PORT, ADDRESS, () => {
  console.log(`\nBW Monitor PWA server running at:`);
  if (ADDRESS === '0.0.0.0') {
    const nets = require('os').networkInterfaces();
    for (const ifaces of Object.values(nets)) {
      for (const iface of ifaces) {
        if (iface.family === 'IPv4' && !iface.internal) {
          console.log(`  http://${iface.address}:${PORT}  (from other devices)`);
        }
      }
    }
    console.log(`  http://localhost:${PORT}  (this machine)`);
  } else {
    console.log(`  http://${ADDRESS}:${PORT}`);
  }
  console.log(`\nOpen this URL on your phone to use the app.`);
});
