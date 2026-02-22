const STORAGE_KEY = "tarsRelayConfig";
const DEFAULT_CONFIG = {
  relayPort: 43182,
  relayToken: "",
  autoReconnect: true,
  reconnectDelayMs: 2000
};

let relaySocket = null;
let reconnectTimer = null;
let activeDebuggee = null;
let activeSessionID = "";
let relayConfig = { ...DEFAULT_CONFIG };
let lastError = "";

const tabBySession = new Map();
const childSessionToTab = new Map();
const tabState = new Map();

function parsePort(value) {
  const n = Number(value);
  if (!Number.isFinite(n) || n <= 0 || n > 65535) {
    return DEFAULT_CONFIG.relayPort;
  }
  return Math.trunc(n);
}

function normalizeRelayToken(value) {
  return String(value || "").trim();
}

function relayHTTPBase(config = relayConfig) {
  return `http://127.0.0.1:${parsePort(config.relayPort)}`;
}

function relayWSURL(path, config = relayConfig) {
  const token = normalizeRelayToken(config.relayToken);
  const url = new URL(path, relayHTTPBase(config).replace("http://", "ws://"));
  if (token) {
    url.searchParams.set("token", token);
  }
  return url.toString();
}

function parseLegacyPort(value) {
  const raw = String(value || "").trim();
  if (!raw) return DEFAULT_CONFIG.relayPort;
  try {
    const parsed = new URL(raw);
    return parsePort(parsed.port || DEFAULT_CONFIG.relayPort);
  } catch (_err) {
    return DEFAULT_CONFIG.relayPort;
  }
}

async function getStoredConfig() {
  const stored = await chrome.storage.local.get(STORAGE_KEY);
  const value = stored?.[STORAGE_KEY] || {};
  const relayPort = parsePort(value.relayPort || parseLegacyPort(value.relayHttpBase));
  return {
    relayPort,
    relayToken: normalizeRelayToken(value.relayToken),
    autoReconnect: value.autoReconnect !== false,
    reconnectDelayMs: Math.max(500, Number(value.reconnectDelayMs || DEFAULT_CONFIG.reconnectDelayMs))
  };
}

async function saveConfig(nextConfig) {
  const resolvedRelayToken = normalizeRelayToken(nextConfig.relayToken || nextConfig.gatewayToken);
  relayConfig = {
    relayPort: parsePort(nextConfig.relayPort),
    relayToken: resolvedRelayToken,
    autoReconnect: nextConfig.autoReconnect !== false,
    reconnectDelayMs: Math.max(500, Number(nextConfig.reconnectDelayMs || DEFAULT_CONFIG.reconnectDelayMs))
  };
  await chrome.storage.local.set({ [STORAGE_KEY]: relayConfig });
  return relayConfig;
}

function setBadge(text, color, title) {
  chrome.action.setBadgeText({ text: String(text || "") }).catch(() => {});
  chrome.action.setBadgeBackgroundColor({ color }).catch(() => {});
  chrome.action.setTitle({ title: String(title || "TARS Relay") }).catch(() => {});
}

function setBadgeDisconnected() {
  setBadge("", "#64748b", "TARS Relay: disconnected");
}

function setBadgeConnecting() {
  setBadge("...", "#ca8a04", "TARS Relay: connecting");
}

function setBadgeConnected() {
  setBadge("ON", "#15803d", "TARS Relay: connected");
}

function setBadgeError(message) {
  const detail = String(message || "error").trim();
  setBadge("!", "#b91c1c", `TARS Relay error: ${detail}`);
}

function clearReconnectTimer() {
  if (!reconnectTimer) return;
  clearTimeout(reconnectTimer);
  reconnectTimer = null;
}

function scheduleReconnect() {
  clearReconnectTimer();
  if (!relayConfig.autoReconnect) return;
  setBadgeConnecting();
  reconnectTimer = setTimeout(() => {
    connectRelay().catch((err) => {
      lastError = String(err?.message || err || "reconnect failed");
      setBadgeError(lastError);
      scheduleReconnect();
    });
  }, Math.max(500, relayConfig.reconnectDelayMs));
}

function clearDebuggeeMappings() {
  tabBySession.clear();
  childSessionToTab.clear();
  tabState.clear();
  activeSessionID = "";
}

async function detachDebuggee() {
  if (!activeDebuggee) {
    clearDebuggeeMappings();
    return;
  }
  try {
    await chrome.debugger.detach(activeDebuggee);
  } catch (_err) {
    // no-op
  } finally {
    activeDebuggee = null;
    clearDebuggeeMappings();
  }
}

function isDebuggableURL(rawURL) {
  const url = String(rawURL || "").trim().toLowerCase();
  if (!url) return true;
  if (url.startsWith("chrome://")) return false;
  if (url.startsWith("chrome-extension://")) return false;
  if (url.startsWith("devtools://")) return false;
  if (url.startsWith("edge://")) return false;
  return true;
}

async function resolveActiveTabDebuggee() {
  const findDebuggableTab = (tabs) => tabs.find((item) => item && typeof item.id === "number" && isDebuggableURL(item.url));
  let tabs = await chrome.tabs.query({ active: true, lastFocusedWindow: true });
  let tab = findDebuggableTab(tabs);
  if (tab?.id != null) return { tabId: tab.id };

  tabs = await chrome.tabs.query({ active: true });
  tab = findDebuggableTab(tabs);
  if (tab?.id != null) return { tabId: tab.id };

  tabs = await chrome.tabs.query({});
  tab = findDebuggableTab(tabs);
  if (tab?.id != null) return { tabId: tab.id };

  throw new Error("no debuggable tab found: open/focus an http(s) tab and retry");
}

async function rememberTabState(tabID) {
  if (typeof tabID !== "number") return;
  try {
    const tab = await chrome.tabs.get(tabID);
    tabState.set(tabID, {
      id: tabID,
      url: String(tab?.url || "").trim(),
      title: String(tab?.title || "").trim()
    });
  } catch (_err) {
    // keep previous state
  }
}

async function sendExtensionReady() {
  if (!activeDebuggee) return;
  await rememberTabState(activeDebuggee.tabId);
  const info = tabState.get(activeDebuggee.tabId) || { url: "", title: "" };
  sendRelayMessage({
    method: "extensionReady",
    params: {
      targetId: String(activeDebuggee.tabId),
      url: String(info.url || "").trim(),
      title: String(info.title || "").trim()
    }
  });
}

async function ensureAttachedDebuggee() {
  if (activeDebuggee) return activeDebuggee;
  const debuggee = await resolveActiveTabDebuggee();
  await chrome.debugger.attach(debuggee, "1.3");
  activeDebuggee = debuggee;
  await rememberTabState(debuggee.tabId);
  await sendExtensionReady();
  return activeDebuggee;
}

function sendRelayMessage(payload) {
  if (!relaySocket || relaySocket.readyState !== WebSocket.OPEN) return;
  try {
    relaySocket.send(JSON.stringify(payload));
  } catch (_err) {
    // no-op
  }
}

function debuggerSessionForCommand(debuggee, sessionID) {
  const base = (debuggee && typeof debuggee === "object") ? debuggee : {};
  const tabID = Number(base.tabId);
  const sid = String(sessionID || "").trim();
  if (!Number.isFinite(tabID) || tabID <= 0) {
    return {};
  }
  if (!sid) {
    return { tabId: tabID };
  }
  // relay handshake uses synthetic session ids (e.g. relay-session-*).
  // Only pass sessionId when extension created/observed a real child debugger session.
  if (!childSessionToTab.has(sid)) {
    return { tabId: tabID };
  }
  return { tabId: tabID, sessionId: sid };
}

async function preflightRelay(config = relayConfig) {
  const token = normalizeRelayToken(config.relayToken);
  if (!token) {
    throw new Error("relay token is required");
  }
  const url = new URL("/json/version", relayHTTPBase(config));
  url.searchParams.set("token", token);
  const response = await fetch(url.toString(), {
    method: "GET",
    headers: {
      "Tars-Relay-Token": token
    }
  });
  if (!response.ok) {
    const body = await response.text();
    throw new Error(`relay preflight failed: ${response.status} ${body}`);
  }
  const payload = await response.json();
  const browser = String(payload?.Browser || "").trim();
  return { ok: true, browser: browser || "Tars Relay", version: browser || "Tars Relay" };
}

function resolveDebuggeeForSession(sessionID) {
  const sid = String(sessionID || "").trim();
  if (!sid) return ensureAttachedDebuggee();
  const knownTab = tabBySession.get(sid) || childSessionToTab.get(sid);
  if (typeof knownTab === "number") {
    return Promise.resolve({ tabId: knownTab });
  }
  return ensureAttachedDebuggee().then((debuggee) => {
    tabBySession.set(sid, debuggee.tabId);
    return debuggee;
  });
}

function onTargetEvent(sourceTabID, method, params) {
  if (typeof sourceTabID === "number") {
    rememberTabState(sourceTabID).catch(() => {});
  }
  if (method === "Target.attachedToTarget") {
    const childSession = String(params?.sessionId || "").trim();
    if (childSession && typeof sourceTabID === "number") {
      childSessionToTab.set(childSession, sourceTabID);
    }
    return;
  }
  if (method === "Target.detachedFromTarget") {
    const childSession = String(params?.sessionId || "").trim();
    if (childSession) {
      childSessionToTab.delete(childSession);
      tabBySession.delete(childSession);
    }
  }
}

async function dispatchForwardCDPCommand(payload) {
  const id = payload?.id;
  const params = (payload && typeof payload.params === "object" && payload.params !== null) ? payload.params : {};
  const method = String(params.method || "").trim();
  const sessionID = String(params.sessionId || "").trim();
  if (!method) {
    sendRelayMessage({
      method: "forwardCDPResponse",
      params: {
        id,
        error: { message: "missing method" },
        sessionId: sessionID
      }
    });
    return;
  }

  let debuggee;
  try {
    debuggee = await resolveDebuggeeForSession(sessionID);
  } catch (err) {
    sendRelayMessage({
      method: "forwardCDPResponse",
      params: {
        id,
        error: { message: String(err?.message || err || "active tab is not debuggable") },
        sessionId: sessionID
      }
    });
    return;
  }

  const cdpParams = (typeof params.params === "object" && params.params !== null) ? params.params : {};
  if (sessionID) {
    activeSessionID = sessionID;
    tabBySession.set(sessionID, debuggee.tabId);
  }

  try {
    const session = debuggerSessionForCommand(debuggee, sessionID);
    const result = await chrome.debugger.sendCommand(session, method, cdpParams);
    if (method === "Target.attachToTarget") {
      const childSession = String(result?.sessionId || "").trim();
      if (childSession && typeof debuggee?.tabId === "number") {
        childSessionToTab.set(childSession, debuggee.tabId);
      }
    }
    sendRelayMessage({
      method: "forwardCDPResponse",
      params: {
        id,
        result: result || {},
        sessionId: sessionID
      }
    });
  } catch (err) {
    sendRelayMessage({
      method: "forwardCDPResponse",
      params: {
        id,
        error: { message: String(err?.message || err || "chrome.debugger command failed") },
        sessionId: sessionID
      }
    });
  }
}

async function dispatchLegacyCDPRequest(payload) {
  const id = payload?.id;
  const method = String(payload?.method || "").trim();
  if (!method) {
    sendRelayMessage({ id, error: { message: "missing method" } });
    return;
  }
  let debuggee;
  try {
    debuggee = await ensureAttachedDebuggee();
  } catch (err) {
    sendRelayMessage({ id, error: { message: String(err?.message || err || "active tab is not debuggable") } });
    return;
  }
  try {
    const cdpParams = (payload && typeof payload.params === "object" && payload.params !== null) ? payload.params : {};
    const result = await chrome.debugger.sendCommand(debuggee, method, cdpParams);
    sendRelayMessage({ id, result: result || {} });
  } catch (err) {
    sendRelayMessage({ id, error: { message: String(err?.message || err || "chrome.debugger command failed") } });
  }
}

function onRelaySocketMessage(event) {
  let payload = null;
  try {
    payload = JSON.parse(event.data);
  } catch (_err) {
    return;
  }
  const method = String(payload?.method || "").trim();
  if (method === "ping") {
    sendRelayMessage({ method: "pong" });
    return;
  }
  if (method === "forwardCDPCommand") {
    dispatchForwardCDPCommand(payload).catch(() => {});
    return;
  }
  dispatchLegacyCDPRequest(payload).catch(() => {});
}

async function connectRelay() {
  clearReconnectTimer();
  if (relaySocket && relaySocket.readyState === WebSocket.OPEN) return;
  if (relaySocket && relaySocket.readyState === WebSocket.CONNECTING) return;

  relayConfig = await getStoredConfig();
  const token = normalizeRelayToken(relayConfig.relayToken);
  if (!token) {
    lastError = "relay token is not configured";
    setBadgeError(lastError);
    return;
  }

  setBadgeConnecting();
  try {
    await preflightRelay(relayConfig);
  } catch (err) {
    lastError = String(err?.message || err || "relay preflight failed");
    setBadgeError(lastError);
    scheduleReconnect();
    return;
  }

  const socket = new WebSocket(relayWSURL("/extension", relayConfig));
  relaySocket = socket;

  socket.addEventListener("open", () => {
    lastError = "";
    setBadgeConnected();
    ensureAttachedDebuggee()
      .then(() => sendExtensionReady())
      .catch(() => {});
  });
  socket.addEventListener("message", onRelaySocketMessage);
  socket.addEventListener("close", () => {
    if (relaySocket === socket) relaySocket = null;
    detachDebuggee().catch(() => {});
    if (!lastError) {
      lastError = "relay socket closed";
    }
    setBadgeError(lastError);
    scheduleReconnect();
  });
  socket.addEventListener("error", () => {
    if (relaySocket === socket) relaySocket = null;
    detachDebuggee().catch(() => {});
    if (!lastError) {
      lastError = "relay socket error";
    }
    setBadgeError(lastError);
    scheduleReconnect();
  });
}

chrome.debugger.onEvent.addListener((source, method, params) => {
  if (!activeDebuggee || source.tabId !== activeDebuggee.tabId) return;
  onTargetEvent(source.tabId, method, params || {});
  sendRelayMessage({
    method: "forwardCDPEvent",
    params: {
      method,
      params: params || {},
      sessionId: activeSessionID
    }
  });
});

chrome.debugger.onDetach.addListener((source) => {
  if (!activeDebuggee) return;
  if (source.tabId === activeDebuggee.tabId) {
    activeDebuggee = null;
    clearDebuggeeMappings();
  }
});

chrome.tabs.onActivated.addListener(() => {
  sendExtensionReady().catch(() => {});
});

chrome.tabs.onUpdated.addListener((tabID, changeInfo) => {
  if (!activeDebuggee || tabID !== activeDebuggee.tabId) return;
  if (typeof changeInfo.url === "string" || typeof changeInfo.title === "string") {
    rememberTabState(tabID).then(() => sendExtensionReady()).catch(() => {});
  }
});

chrome.runtime.onInstalled.addListener(async () => {
  await saveConfig(await getStoredConfig());
  connectRelay().catch((err) => {
    lastError = String(err?.message || err || "connect failed");
    setBadgeError(lastError);
    scheduleReconnect();
  });
});

chrome.runtime.onStartup.addListener(() => {
  connectRelay().catch((err) => {
    lastError = String(err?.message || err || "connect failed");
    setBadgeError(lastError);
    scheduleReconnect();
  });
});

chrome.action.onClicked.addListener(() => {
  if (relaySocket && relaySocket.readyState === WebSocket.OPEN) {
    relaySocket.close();
    relaySocket = null;
    detachDebuggee().catch(() => {});
    setBadgeDisconnected();
    return;
  }
  connectRelay().catch((err) => {
    lastError = String(err?.message || err || "connect failed");
    setBadgeError(lastError);
    scheduleReconnect();
  });
});

chrome.runtime.onMessage.addListener((message, _sender, sendResponse) => {
  const type = String(message?.type || "").trim();
  if (type === "tarsRelay.getStatus") {
    sendResponse({
      connected: !!relaySocket && relaySocket.readyState === WebSocket.OPEN,
      relayPort: parsePort(relayConfig.relayPort),
      relayToken: normalizeRelayToken(relayConfig.relayToken),
      tokenSet: normalizeRelayToken(relayConfig.relayToken) !== "",
      autoReconnect: relayConfig.autoReconnect !== false,
      reconnectDelayMs: Math.max(500, Number(relayConfig.reconnectDelayMs || DEFAULT_CONFIG.reconnectDelayMs)),
      debuggeeTabId: activeDebuggee?.tabId || null,
      lastError
    });
    return;
  }
  if (type === "tarsRelay.preflight") {
    preflightRelay(relayConfig)
      .then((result) => sendResponse({ ok: true, ...result }))
      .catch((err) => sendResponse({ ok: false, error: String(err?.message || err) }));
    return true;
  }
  if (type === "tarsRelay.setConfig") {
    saveConfig({
      relayPort: parsePort(message?.relayPort),
      relayToken: normalizeRelayToken(message?.relayToken || message?.gatewayToken),
      autoReconnect: message?.autoReconnect !== false,
      reconnectDelayMs: Math.max(500, Number(message?.reconnectDelayMs || DEFAULT_CONFIG.reconnectDelayMs))
    })
      .then((next) => {
        if (relaySocket && relaySocket.readyState === WebSocket.OPEN) {
          relaySocket.close();
        }
        lastError = "";
        connectRelay().catch((err) => {
          lastError = String(err?.message || err || "connect failed");
          setBadgeError(lastError);
          scheduleReconnect();
        });
        sendResponse({ ok: true, config: next });
      })
      .catch((err) => {
        sendResponse({ ok: false, error: String(err?.message || err) });
      });
    return true;
  }
});

setBadgeDisconnected();
