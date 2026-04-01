// services.js - Kubeconfig parser, Kubernetes client, and monitor service
// for the BunkerWeb monitoring PWA.

// ---------------------------------------------------------------------------
// Minimal YAML parser
// ---------------------------------------------------------------------------
// Handles the subset of YAML used by kubeconfig files: mappings, sequences,
// scalars (plain, single-quoted, double-quoted), booleans, nulls.
// No anchors, aliases, tags, multi-line blocks, or flow collections.

(function () {
  'use strict';

  function parseYAML(text) {
    var lines = text.replace(/\r\n/g, '\n').split('\n');
    var idx = 0;

    function peekIndent() {
      while (idx < lines.length) {
        var line = lines[idx];
        // skip blank lines and full-line comments
        if (/^\s*$/.test(line) || /^\s*#/.test(line)) { idx++; continue; }
        var m = line.match(/^(\s*)/);
        return m[1].length;
      }
      return -1;
    }

    function currentLine() {
      return idx < lines.length ? lines[idx] : null;
    }

    function parseScalar(raw) {
      if (raw === undefined || raw === null) return null;
      raw = raw.trim();
      if (raw === '' || raw === '~' || raw === 'null') return null;
      if (raw === 'true') return true;
      if (raw === 'false') return false;
      // strip quotes
      if ((raw[0] === '"' && raw[raw.length - 1] === '"') ||
          (raw[0] === "'" && raw[raw.length - 1] === "'")) {
        return raw.slice(1, -1);
      }
      // number
      if (/^-?\d+$/.test(raw)) return parseInt(raw, 10);
      if (/^-?\d+\.\d+$/.test(raw)) return parseFloat(raw);
      return raw;
    }

    function parseValue(raw, baseIndent) {
      raw = raw.trim();
      // remove inline comment (not inside quotes)
      if (raw[0] !== '"' && raw[0] !== "'") {
        var commentIdx = raw.indexOf(' #');
        if (commentIdx >= 0) raw = raw.slice(0, commentIdx).trim();
      }
      if (raw === '' || raw === '~' || raw === 'null') {
        // could be a nested mapping or sequence on next lines
        var ni = peekIndent();
        if (ni > baseIndent) {
          return parseBlock(ni);
        }
        return null;
      }
      return parseScalar(raw);
    }

    function parseBlock(indent) {
      var ni = peekIndent();
      if (ni < 0) return null;
      var line = currentLine();
      var trimmed = line.trim();

      // Is this a sequence?
      if (trimmed.startsWith('- ') || trimmed === '-') {
        return parseSequence(indent);
      }
      // Otherwise a mapping
      return parseMapping(indent);
    }

    function parseMapping(indent) {
      var obj = {};
      while (idx < lines.length) {
        var ni = peekIndent();
        if (ni < 0 || ni < indent) break;
        if (ni > indent) break; // shouldn't happen for same-level keys

        var line = lines[idx];
        var trimmed = line.trim();

        // handle sequence item containing a mapping (e.g. "- name: foo")
        if (trimmed.startsWith('- ')) break;

        var colonIdx = trimmed.indexOf(':');
        if (colonIdx < 0) { idx++; continue; }

        var key = trimmed.slice(0, colonIdx).trim();
        var rest = trimmed.slice(colonIdx + 1);
        idx++;

        obj[key] = parseValue(rest, indent);
      }
      return obj;
    }

    function parseSequence(indent) {
      var arr = [];
      while (idx < lines.length) {
        var ni = peekIndent();
        if (ni < 0 || ni < indent) break;
        if (ni > indent) break;

        var line = lines[idx];
        var trimmed = line.trim();
        if (!trimmed.startsWith('- ')) break;

        var after = trimmed.slice(2);
        idx++;

        // "- key: value" -> mapping item
        if (after.indexOf(':') >= 0 && !after.startsWith('"') && !after.startsWith("'")) {
          // The first key-value is part of a mapping; parse the rest at deeper indent
          var item = {};
          var firstColon = after.indexOf(':');
          var k = after.slice(0, firstColon).trim();
          var v = after.slice(firstColon + 1);

          var childIndent = indent + 2;
          item[k] = parseValue(v, childIndent);

          // read remaining keys at child indent
          while (idx < lines.length) {
            var ci = peekIndent();
            if (ci < childIndent) break;
            if (ci > childIndent) {
              // deeper nesting handled by parseValue
              break;
            }
            var cl = lines[idx].trim();
            if (cl.startsWith('- ')) break;
            var cc = cl.indexOf(':');
            if (cc < 0) { idx++; continue; }
            var ck = cl.slice(0, cc).trim();
            var cv = cl.slice(cc + 1);
            idx++;
            item[ck] = parseValue(cv, childIndent);
          }
          arr.push(item);
        } else {
          arr.push(parseScalar(after));
        }
      }
      return arr;
    }

    // Skip leading document markers
    while (idx < lines.length && /^\s*---\s*$/.test(lines[idx])) idx++;

    var ni = peekIndent();
    if (ni < 0) return null;
    return parseBlock(ni);
  }

  // ---------------------------------------------------------------------------
  // KubeConfig
  // ---------------------------------------------------------------------------

  function KubeConfig() {}

  /**
   * List all available contexts from a kubeconfig YAML string.
   * Returns { contexts: [{name, cluster, user, namespace}], currentContext: string }
   */
  KubeConfig.listContexts = function (yamlString) {
    var doc = parseYAML(yamlString);
    if (!doc) throw new Error('Failed to parse kubeconfig YAML');

    var currentContext = doc['current-context'] || '';
    var contexts = (doc.contexts || []).map(function (entry) {
      var ctx = entry.context || {};
      return {
        name: entry.name || '',
        cluster: ctx.cluster || '',
        user: ctx.user || '',
        namespace: ctx.namespace || ''
      };
    });

    return { contexts: contexts, currentContext: currentContext };
  };

  /**
   * Parse a kubeconfig YAML string and resolve a context into a flat
   * config object: { server, caCertData, token, username, password,
   * insecureSkipTLSVerify, namespace }.
   *
   * @param {string} yamlString
   * @param {string} [contextName] - Context to use. If omitted, uses current-context.
   */
  KubeConfig.parse = function (yamlString, contextName) {
    var doc = parseYAML(yamlString);
    if (!doc) throw new Error('Failed to parse kubeconfig YAML');

    var ctxName = contextName || doc['current-context'];
    if (!ctxName) throw new Error('No context specified and no current-context in kubeconfig');

    // Find context entry
    var contexts = doc.contexts || [];
    var ctxEntry = null;
    for (var i = 0; i < contexts.length; i++) {
      if (contexts[i].name === ctxName) { ctxEntry = contexts[i]; break; }
    }
    if (!ctxEntry) throw new Error('Context "' + ctxName + '" not found');
    var ctx = ctxEntry.context || {};

    var clusterName = ctx.cluster;
    var userName = ctx.user;
    var namespace = ctx.namespace || 'default';

    // Find cluster entry
    var clusters = doc.clusters || [];
    var clusterEntry = null;
    for (var i = 0; i < clusters.length; i++) {
      if (clusters[i].name === clusterName) { clusterEntry = clusters[i]; break; }
    }
    if (!clusterEntry) throw new Error('Cluster "' + clusterName + '" not found');
    var cluster = clusterEntry.cluster || {};

    // Find user entry
    var users = doc.users || [];
    var userEntry = null;
    for (var i = 0; i < users.length; i++) {
      if (users[i].name === userName) { userEntry = users[i]; break; }
    }
    var user = userEntry ? (userEntry.user || {}) : {};

    return {
      server: cluster.server || '',
      caCertData: cluster['certificate-authority-data'] || null,
      token: user.token || null,
      username: user.username || null,
      password: user.password || null,
      insecureSkipTLSVerify: cluster['insecure-skip-tls-verify'] === true,
      namespace: namespace
    };
  };

  window.KubeConfig = KubeConfig;

  // ---------------------------------------------------------------------------
  // KubernetesClient
  // ---------------------------------------------------------------------------

  /**
   * @param {Object} config - Parsed kubeconfig object from KubeConfig.parse()
   */
  function KubernetesClient(config) {
    this.server = (config.server || '').replace(/\/+$/, '');
    this.token = config.token || null;
    this.username = config.username || null;
    this.password = config.password || null;
    this.caCertData = config.caCertData || null;
    this.insecureSkipTLSVerify = !!config.insecureSkipTLSVerify;
  }

  /**
   * Build authorization headers for API requests.
   */
  KubernetesClient.prototype._authHeaders = function () {
    var headers = {};
    if (this.token) {
      headers['Authorization'] = 'Bearer ' + this.token;
    } else if (this.username && this.password) {
      headers['Authorization'] = 'Basic ' + btoa(this.username + ':' + this.password);
    }
    return headers;
  };

  /**
   * Perform an authenticated fetch against the K8s API.
   * NOTE: Browser fetch cannot inject custom CA certificates. If the cluster
   * uses a self-signed CA the user must either trust it at the OS/browser
   * level or access the API through a proxy that terminates TLS.
   */
  KubernetesClient.prototype._fetch = function (path, opts) {
    opts = opts || {};
    var headers = Object.assign(this._authHeaders(), opts.headers || {});
    return fetch(this.server + path, Object.assign({}, opts, { headers: headers }));
  };

  /**
   * List pods in a namespace filtered by label selector.
   * @returns {Promise<Array<{name:string, phase:string}>>}
   */
  KubernetesClient.prototype.listPods = async function (namespace, labelSelector) {
    var qs = labelSelector ? '?labelSelector=' + encodeURIComponent(labelSelector) : '';
    var resp = await this._fetch('/api/v1/namespaces/' + encodeURIComponent(namespace) + '/pods' + qs);
    if (!resp.ok) {
      throw new Error('listPods failed: ' + resp.status + ' ' + resp.statusText);
    }
    var body = await resp.json();
    return (body.items || []).map(function (item) {
      return {
        name: item.metadata.name,
        phase: item.status.phase
      };
    });
  };

  /**
   * Find the first Running pod with label bunkerweb.io/component=redis.
   * @returns {Promise<string>} Pod name
   */
  KubernetesClient.prototype.findRedisPod = async function (namespace) {
    var pods = await this.listPods(namespace, 'bunkerweb.io/component=redis');
    for (var i = 0; i < pods.length; i++) {
      if (pods[i].phase === 'Running') return pods[i].name;
    }
    throw new Error('No running Redis pod found with label bunkerweb.io/component=redis');
  };

  /**
   * Execute a command in a pod via the K8s exec WebSocket API.
   * Uses subprotocol v4.channel.k8s.io.
   *
   * @param {string} namespace
   * @param {string} pod
   * @param {string[]} command - Array of command arguments
   * @returns {Promise<string>} stdout output
   */
  KubernetesClient.prototype.exec = function (namespace, pod, command) {
    var self = this;
    var TIMEOUT_MS = 15000;

    return new Promise(function (resolve, reject) {
      // Build WebSocket URL
      var wsBase = self.server
        .replace(/^https:\/\//, 'wss://')
        .replace(/^http:\/\//, 'ws://');
      var path = '/api/v1/namespaces/' + encodeURIComponent(namespace) +
                 '/pods/' + encodeURIComponent(pod) + '/exec';

      var params = [];
      for (var i = 0; i < command.length; i++) {
        params.push('command=' + encodeURIComponent(command[i]));
      }
      params.push('stdout=true');
      params.push('stderr=true');

      var url = wsBase + path + '?' + params.join('&');

      // Add auth token as a query parameter for WebSocket if using bearer token,
      // since WebSocket API does not support custom headers in the browser.
      // Some K8s API servers accept access_token as a query param.
      // For basic auth, the credentials can be embedded in the URL.
      var protocols = ['v4.channel.k8s.io'];

      // NOTE: Browser WebSocket does not support custom headers. For bearer
      // token auth, the token must be passed via a supported mechanism (e.g.
      // a proxy that injects the header, or the base64.bearer.authorization
      // subprotocol trick used by some K8s dashboard implementations).
      // We use the base64-encoded bearer token as a subprotocol.
      if (self.token) {
        protocols.push('base64url.bearer.authorization.k8s.io.' + btoa(self.token));
      }

      var ws;
      if (self.username && self.password) {
        // Embed basic auth credentials in the URL
        var urlObj = new URL(url);
        urlObj.username = self.username;
        urlObj.password = self.password;
        ws = new WebSocket(urlObj.toString(), protocols);
      } else {
        ws = new WebSocket(url, protocols);
      }

      ws.binaryType = 'arraybuffer';

      var stdout = '';
      var timer = setTimeout(function () {
        ws.close();
        reject(new Error('exec timed out after ' + TIMEOUT_MS + 'ms'));
      }, TIMEOUT_MS);

      ws.onopen = function () {
        // Connection established; wait for data.
      };

      ws.onmessage = function (event) {
        if (!(event.data instanceof ArrayBuffer)) return;
        var buf = new Uint8Array(event.data);
        if (buf.length < 1) return;

        var channel = buf[0];
        var payload = new TextDecoder().decode(buf.slice(1));

        if (channel === 1) {
          // stdout
          stdout += payload;
        } else if (channel === 3) {
          // status channel - command finished
          clearTimeout(timer);
          ws.close();
          resolve(stdout);
        }
        // channel 2 = stderr, ignored for our purposes
      };

      ws.onerror = function (err) {
        clearTimeout(timer);
        reject(new Error('WebSocket error during exec: ' + (err.message || 'unknown')));
      };

      ws.onclose = function (event) {
        clearTimeout(timer);
        // If we haven't resolved yet, resolve with whatever we have
        resolve(stdout);
      };
    });
  };

  window.KubernetesClient = KubernetesClient;

  // ---------------------------------------------------------------------------
  // MonitorService
  // ---------------------------------------------------------------------------

  /**
   * @param {KubernetesClient} client
   * @param {string} namespace
   */
  function MonitorService(client, namespace) {
    this.client = client;
    this.namespace = namespace;
    this.redisPod = null;
    this.highwater = 0;
  }

  /**
   * Run a redis-cli command in the Redis pod via exec.
   * @param {string[]} args - Arguments to redis-cli
   * @returns {Promise<string>}
   */
  MonitorService.prototype._redis = function (args) {
    var command = ['redis-cli', '--raw'].concat(args);
    return this.client.exec(this.namespace, this.redisPod, command);
  };

  /**
   * Run a Lua script via redis-cli EVAL.
   * @param {string} script - Lua script
   * @param {number} numkeys - Number of KEYS arguments
   * @param {string[]} args - KEYS and ARGV arguments
   * @returns {Promise<string>}
   */
  MonitorService.prototype._eval = function (script, numkeys, args) {
    var cmd = ['EVAL', script, String(numkeys)].concat(args);
    return this._redis(cmd);
  };

  /**
   * Discover the Redis pod and verify connectivity with PING.
   * @returns {Promise<string>} Pod name
   */
  MonitorService.prototype.connect = async function () {
    this.redisPod = await this.client.findRedisPod(this.namespace);

    var pong = await this._redis(['PING']);
    if (pong.trim() !== 'PONG') {
      throw new Error('Redis PING failed, got: ' + pong.trim());
    }

    return this.redisPod;
  };

  /**
   * Parse a single report JSON string into an object.
   */
  function parseReport(jsonStr) {
    try {
      return JSON.parse(jsonStr);
    } catch (e) {
      return null;
    }
  }

  /**
   * Load the initial set of blocked-request reports from Redis.
   * @param {number} maxEntries - Maximum number of entries to load (0 = all)
   * @returns {Promise<{reports: Array, total: number}>}
   */
  MonitorService.prototype.loadInitial = async function (maxEntries) {
    maxEntries = maxEntries || 0;

    var script =
      "local len = redis.call('llen', 'requests')\n" +
      "local max = tonumber(ARGV[1])\n" +
      "local start = 0\n" +
      "if max > 0 and len > max then start = len - max end\n" +
      "if len == 0 then return '0' end\n" +
      "local items = redis.call('lrange', 'requests', start, -1)\n" +
      "if #items == 0 then return tostring(len) end\n" +
      "return tostring(len) .. '\\n' .. table.concat(items, '\\n')";

    var raw = await this._eval(script, 0, [String(maxEntries)]);
    raw = raw.trim();

    var lines = raw.split('\n');
    var total = parseInt(lines[0], 10) || 0;
    this.highwater = total;

    var reports = [];
    for (var i = 1; i < lines.length; i++) {
      var line = lines[i].trim();
      if (!line) continue;
      var r = parseReport(line);
      if (r) reports.push(r);
    }

    // Reverse so newest first
    reports.reverse();

    return { reports: reports, total: total };
  };

  /**
   * Poll for new reports since last highwater mark.
   * @returns {Promise<Array>} New reports, newest first
   */
  MonitorService.prototype.pollNew = async function () {
    var script =
      "local len = redis.call('llen', 'requests')\n" +
      "if len <= tonumber(ARGV[1]) then return tostring(len) end\n" +
      "local items = redis.call('lrange', 'requests', tonumber(ARGV[1]), -1)\n" +
      "if #items == 0 then return tostring(len) end\n" +
      "return tostring(len) .. '\\n' .. table.concat(items, '\\n')";

    var raw = await this._eval(script, 0, [String(this.highwater)]);
    raw = raw.trim();

    var lines = raw.split('\n');
    var total = parseInt(lines[0], 10) || 0;
    this.highwater = total;

    var reports = [];
    for (var i = 1; i < lines.length; i++) {
      var line = lines[i].trim();
      if (!line) continue;
      var r = parseReport(line);
      if (r) reports.push(r);
    }

    reports.reverse();
    return reports;
  };

  /**
   * Load current bans from Redis.
   * @returns {Promise<Array<{ip:string, key:string, ttl:number, data:Object}>>}
   */
  MonitorService.prototype.loadBans = async function () {
    var script =
      "local keys = redis.call('keys', 'bans_*')\n" +
      "if #keys == 0 then return '' end\n" +
      "local r = {}\n" +
      "for _, k in ipairs(keys) do\n" +
      "  local v = redis.call('get', k)\n" +
      "  local t = redis.call('ttl', k)\n" +
      "  if v then r[#r+1] = k .. '\\t' .. t .. '\\t' .. v end\n" +
      "end\n" +
      "return table.concat(r, '\\n')";

    var raw = await this._eval(script, 0, []);
    raw = raw.trim();
    if (!raw) return [];

    var lines = raw.split('\n');
    var bans = [];

    for (var i = 0; i < lines.length; i++) {
      var line = lines[i].trim();
      if (!line) continue;

      var parts = line.split('\t');
      if (parts.length < 3) continue;

      var key = parts[0];
      var ttl = parseInt(parts[1], 10);
      var json = parts.slice(2).join('\t');

      // Parse IP from key: bans_ip_1.2.3.4 -> 1.2.3.4
      var ip = '';
      var ipIdx = key.indexOf('_ip_');
      if (ipIdx >= 0) {
        ip = key.slice(ipIdx + 4);
      }

      var data = null;
      try { data = JSON.parse(json); } catch (e) { data = { raw: json }; }

      bans.push({
        ip: ip,
        key: key,
        ttl: ttl,
        data: data
      });
    }

    // Sort by date descending (use data.date if present, otherwise by key)
    bans.sort(function (a, b) {
      var da = (a.data && a.data.date) || 0;
      var db = (b.data && b.data.date) || 0;
      return db - da;
    });

    return bans;
  };

  window.MonitorService = MonitorService;

  // ---------------------------------------------------------------------------
  // Utility functions
  // ---------------------------------------------------------------------------

  var IP_PALETTE = [
    '#FF6B6B', '#4EC9B0', '#DCDCAA', '#569CD6', '#C586C0',
    '#4FC1FF', '#CE9178', '#B5CEA8', '#D7BA7D', '#9CDCFE',
    '#F48771', '#7EC8E3', '#C3E88D', '#F78C6C', '#FF79C6',
    '#8BE9FD', '#50FA7B', '#FFB86C', '#BD93F9', '#F1FA8C'
  ];

  /**
   * Deterministic color for an IP address using FNV-1a hash.
   * @param {string} ip
   * @returns {string} Hex color string
   */
  window.ipColor = function (ip) {
    // FNV-1a 32-bit
    var hash = 0x811c9dc5;
    for (var i = 0; i < ip.length; i++) {
      hash ^= ip.charCodeAt(i);
      hash = Math.imul(hash, 0x01000193);
    }
    var index = (hash >>> 0) % IP_PALETTE.length;
    return IP_PALETTE[index];
  };

  /**
   * Convert a 2-letter country code to a flag emoji.
   * Uses regional indicator symbols: each letter maps to U+1F1E6..U+1F1FF.
   * @param {string} countryCode - Two-letter ISO 3166-1 alpha-2 code
   * @returns {string} Flag emoji
   */
  window.flagEmoji = function (countryCode) {
    if (!countryCode || countryCode.length !== 2) return '';
    var cc = countryCode.toUpperCase();
    return String.fromCodePoint(
      cc.charCodeAt(0) + 127397,
      cc.charCodeAt(1) + 127397
    );
  };

  /**
   * Format a Unix timestamp as "HH:MM:SS".
   * @param {number} ts - Unix timestamp in seconds
   * @returns {string}
   */
  window.formatTime = function (ts) {
    var d = new Date(ts * 1000);
    var h = String(d.getHours()).padStart(2, '0');
    var m = String(d.getMinutes()).padStart(2, '0');
    var s = String(d.getSeconds()).padStart(2, '0');
    return h + ':' + m + ':' + s;
  };

  /**
   * Format a Unix timestamp as "YYYY-MM-DD HH:MM:SS".
   * @param {number} ts - Unix timestamp in seconds
   * @returns {string}
   */
  window.formatDateTime = function (ts) {
    var d = new Date(ts * 1000);
    var y = d.getFullYear();
    var mo = String(d.getMonth() + 1).padStart(2, '0');
    var da = String(d.getDate()).padStart(2, '0');
    var h = String(d.getHours()).padStart(2, '0');
    var m = String(d.getMinutes()).padStart(2, '0');
    var s = String(d.getSeconds()).padStart(2, '0');
    return y + '-' + mo + '-' + da + ' ' + h + ':' + m + ':' + s;
  };

})();
