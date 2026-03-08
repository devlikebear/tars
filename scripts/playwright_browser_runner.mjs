import fs from 'node:fs';
import path from 'node:path';
import process from 'node:process';
import { chromium } from 'playwright';

const raw = fs.readFileSync(0, 'utf8');
const req = JSON.parse(raw);

function fail(message) {
  process.stderr.write(String(message).trim() + '\n');
  process.exit(1);
}

function hostAllowed(rawUrl, allowlist) {
  if (!rawUrl || !allowlist || allowlist.length === 0) return true;
  let hostname = '';
  try {
    hostname = new URL(rawUrl).hostname.toLowerCase().trim();
  } catch (error) {
    fail(`invalid url: ${rawUrl}`);
  }
  if (!hostname) return false;
  for (const pattern of allowlist) {
    const candidate = String(pattern || '').trim().toLowerCase();
    if (!candidate) continue;
    if (candidate === hostname) return true;
    if (candidate.startsWith('*.')) {
      const suffix = candidate.slice(2);
      if (suffix && (hostname === suffix || hostname.endsWith(`.${suffix}`))) return true;
    }
  }
  return false;
}

function ensureAllowed(rawUrl, allowlist) {
  if (!hostAllowed(rawUrl, allowlist)) {
    fail(`allowed_hosts policy blocked ${rawUrl}`);
  }
}

function waitMillis(value) {
  const parsed = Number(value || 0);
  if (!Number.isFinite(parsed) || parsed <= 0) return 1000;
  return parsed;
}

function ensureDir(target) {
  if (!target) return;
  fs.mkdirSync(path.dirname(target), { recursive: true });
}

async function withPage(req, fn) {
  const launchOptions = {
    headless: req.headless !== false,
  };
  if (req.executable_path) {
    launchOptions.executablePath = req.executable_path;
  }
  const userDataDir = req.user_data_dir || path.join(process.cwd(), 'workspace', '_shared', 'browser', 'managed', 'default');
  fs.mkdirSync(userDataDir, { recursive: true });
  const context = await chromium.launchPersistentContext(userDataDir, launchOptions);
  try {
    const page = context.pages()[0] || await context.newPage();
    return await fn(context, page);
  } finally {
    await context.close();
  }
}

async function handleLogin(req) {
  const login = req.login || {};
  const url = String(req.url || '').trim();
  if (!url) fail('login url is required');
  ensureAllowed(url, req.allowed_hosts || []);
  const creds = req.credentials || {};
  await withPage(req, async (_context, page) => {
    await page.goto(url, { waitUntil: 'domcontentloaded' });
    if (login.username_selector) {
      await page.fill(login.username_selector, String(creds.username || ''));
    }
    if (login.password_selector) {
      await page.fill(login.password_selector, String(creds.password || ''));
    }
    if (login.submit_selector) {
      await page.click(login.submit_selector);
    }
    if (login.success_selector) {
      await page.waitForSelector(login.success_selector, { timeout: waitMillis(login.timeout_ms || 15000) });
    }
    process.stdout.write(JSON.stringify({
      current_url: page.url(),
      message: 'auto login completed',
    }));
  });
}

async function handleCheck(req) {
  const url = String(req.url || '').trim();
  if (!url) fail('check url is required');
  ensureAllowed(url, req.allowed_hosts || []);
  await withPage(req, async (_context, page) => {
    await page.goto(url, { waitUntil: 'domcontentloaded' });
    for (const check of req.checks || []) {
      if (check.selector) {
        await page.waitForSelector(check.selector, { timeout: waitMillis(check.timeout_ms || 5000) });
        if (check.contains) {
          const text = await page.textContent(check.selector);
          if (!String(text || '').includes(String(check.contains))) {
            fail(`selector ${check.selector} missing expected text`);
          }
        }
      }
    }
    const snapshot = await page.evaluate(() => String(document?.body?.innerText || '').trim().slice(0, 1000));
    process.stdout.write(JSON.stringify({
      current_url: page.url(),
      snapshot,
      passed: true,
      message: `checks passed (${(req.checks || []).length})`,
    }));
  });
}

async function handleRun(req) {
  await withPage(req, async (_context, page) => {
    if (req.url) {
      ensureAllowed(req.url, req.allowed_hosts || []);
      await page.goto(req.url, { waitUntil: 'domcontentloaded' });
    }
    let lastAction = '';
    for (const step of req.steps || []) {
      if (step.open) {
        ensureAllowed(step.open, req.allowed_hosts || []);
        await page.goto(step.open, { waitUntil: 'domcontentloaded' });
        lastAction = `open ${step.open}`;
        continue;
      }
      if (step.click) {
        await page.click(step.click);
        lastAction = `click ${step.click}`;
        continue;
      }
      if (step.type) {
        await page.fill(step.type, String(step.value || ''));
        lastAction = `type ${step.type}`;
        continue;
      }
      if (step.wait) {
        await page.waitForTimeout(waitMillis(step.wait));
        lastAction = `wait ${step.wait}`;
        continue;
      }
      if (step.extract) {
        await page.waitForSelector(step.extract, { timeout: 5000 });
        const text = await page.textContent(step.extract);
        lastAction = `extract ${step.extract}`;
        if (text) {
          lastAction += ` => ${String(text).trim().slice(0, 120)}`;
        }
      }
    }
    process.stdout.write(JSON.stringify({
      current_url: page.url(),
      last_action: lastAction,
      message: `executed ${(req.steps || []).length} steps`,
      passed: true,
    }));
  });
}

async function handleOpen(req) {
  const url = String(req.url || '').trim();
  if (!url) fail('open url is required');
  ensureAllowed(url, req.allowed_hosts || []);
  await withPage(req, async (_context, page) => {
    await page.goto(url, { waitUntil: 'domcontentloaded' });
    process.stdout.write(JSON.stringify({ current_url: page.url(), last_action: `open ${page.url()}` }));
  });
}

async function handleSnapshot(req) {
  const url = String(req.url || '').trim();
  if (!url) fail('snapshot url is required');
  ensureAllowed(url, req.allowed_hosts || []);
  await withPage(req, async (_context, page) => {
    await page.goto(url, { waitUntil: 'domcontentloaded' });
    const snapshot = await page.evaluate(() => String(document?.body?.innerText || '').trim().slice(0, 1000));
    process.stdout.write(JSON.stringify({ current_url: page.url(), snapshot }));
  });
}

async function handleAct(req) {
  const url = String(req.url || '').trim();
  if (!url) fail('act url is required');
  ensureAllowed(url, req.allowed_hosts || []);
  await withPage(req, async (_context, page) => {
    await page.goto(url, { waitUntil: 'domcontentloaded' });
    const action = String(req.action || '').trim().toLowerCase();
    if (action === 'click') {
      await page.click(String(req.target || '').trim());
    } else if (action === 'type') {
      await page.fill(String(req.target || '').trim(), String(req.value || ''));
    } else if (action === 'wait') {
      if (req.target) {
        await page.waitForSelector(String(req.target || '').trim(), { timeout: waitMillis(req.value || 5000) });
      } else {
        await page.waitForTimeout(waitMillis(req.value || 1000));
      }
    } else if (action === 'evaluate') {
      const output = await page.evaluate(String(req.value || req.target || ''));
      process.stdout.write(JSON.stringify({
        current_url: page.url(),
        snapshot: String(output ?? ''),
        last_action: `evaluate ${String(req.target || '').trim()}`,
      }));
      return;
    } else {
      fail(`unsupported action: ${action}`);
    }
    process.stdout.write(JSON.stringify({
      current_url: page.url(),
      last_action: `${action} ${String(req.target || '').trim()}`.trim(),
    }));
  });
}

async function handleScreenshot(req) {
  const url = String(req.url || '').trim();
  if (!url) fail('screenshot url is required');
  const screenshotPath = String(req.screenshot_path || '').trim();
  if (!screenshotPath) fail('screenshot path is required');
  ensureAllowed(url, req.allowed_hosts || []);
  ensureDir(screenshotPath);
  await withPage(req, async (_context, page) => {
    await page.goto(url, { waitUntil: 'domcontentloaded' });
    await page.screenshot({ path: screenshotPath, fullPage: true });
    process.stdout.write(JSON.stringify({
      current_url: page.url(),
      screenshot_path: screenshotPath,
      last_action: `screenshot ${screenshotPath}`,
    }));
  });
}

const mode = String(req.mode || '').trim().toLowerCase();
try {
  switch (mode) {
    case 'login':
      await handleLogin(req);
      break;
    case 'check':
      await handleCheck(req);
      break;
    case 'run':
      await handleRun(req);
      break;
    case 'open':
      await handleOpen(req);
      break;
    case 'snapshot':
      await handleSnapshot(req);
      break;
    case 'act':
      await handleAct(req);
      break;
    case 'screenshot':
      await handleScreenshot(req);
      break;
    default:
      fail(`unsupported mode: ${mode}`);
  }
} catch (error) {
  fail(error instanceof Error ? error.message : String(error));
}
