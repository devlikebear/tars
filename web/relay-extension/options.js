const elPort = document.getElementById("relay-port");
const elToken = document.getElementById("relay-token");
const elAutoReconnect = document.getElementById("auto-reconnect");
const elReconnectDelay = document.getElementById("reconnect-delay");
const elSave = document.getElementById("save-btn");
const elPreflight = document.getElementById("preflight-btn");
const elStatus = document.getElementById("status");

function setStatus(message, isError = false) {
  elStatus.textContent = String(message || "").trim();
  if (isError) {
    elStatus.classList.add("error");
  } else {
    elStatus.classList.remove("error");
  }
}

function parsePositiveInt(raw, fallback) {
  const n = Number(raw);
  if (!Number.isFinite(n) || n <= 0) return fallback;
  return Math.trunc(n);
}

async function getStatus() {
  const response = await chrome.runtime.sendMessage({ type: "tarsRelay.getStatus" });
  return response || {};
}

async function applyConfig(payload) {
  const response = await chrome.runtime.sendMessage({
    type: "tarsRelay.setConfig",
    relayPort: payload.relayPort,
    relayToken: payload.relayToken,
    autoReconnect: payload.autoReconnect,
    reconnectDelayMs: payload.reconnectDelayMs
  });
  if (!response?.ok) {
    throw new Error(String(response?.error || "failed to save relay config"));
  }
  return response.config || {};
}

async function runPreflight() {
  const response = await chrome.runtime.sendMessage({ type: "tarsRelay.preflight" });
  if (!response?.ok) {
    throw new Error(String(response?.error || "relay preflight failed"));
  }
  return response;
}

function renderConfig(status) {
  const relayPort = parsePositiveInt(status.relayPort, 43182);
  elPort.value = String(relayPort);
  elToken.value = String(status.relayToken || "");
  elAutoReconnect.checked = status.autoReconnect !== false;
  elReconnectDelay.value = String(parsePositiveInt(status.reconnectDelayMs, 2000));
}

function buildPayload() {
  return {
    relayPort: parsePositiveInt(elPort.value, 43182),
    relayToken: String(elToken.value || "").trim(),
    autoReconnect: !!elAutoReconnect.checked,
    reconnectDelayMs: parsePositiveInt(elReconnectDelay.value, 2000)
  };
}

async function refreshStatusLine() {
  const status = await getStatus();
  const details = [
    `connected=${!!status.connected}`,
    `relay=http://127.0.0.1:${parsePositiveInt(status.relayPort, 43182)}`,
    `token_set=${!!status.tokenSet}`,
    `debuggee_tab=${status.debuggeeTabId ?? "none"}`
  ];
  if (status.lastError) {
    details.push(`last_error=${String(status.lastError).trim()}`);
  }
  setStatus(details.join(" | "), !!status.lastError);
}

async function init() {
  const status = await getStatus();
  renderConfig(status);
  await refreshStatusLine();
}

elSave.addEventListener("click", async () => {
  try {
    await applyConfig(buildPayload());
    await refreshStatusLine();
    setStatus("Saved relay settings successfully.");
  } catch (error) {
    setStatus(error?.message || String(error), true);
  }
});

elPreflight.addEventListener("click", async () => {
  try {
    const result = await runPreflight();
    setStatus(`Relay check passed. ${String(result.version || "").trim()}`);
  } catch (error) {
    setStatus(error?.message || String(error), true);
  }
});

init().catch((error) => {
  setStatus(error?.message || String(error), true);
});
