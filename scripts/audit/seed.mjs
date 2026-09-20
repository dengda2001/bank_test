#!/usr/bin/env node
// Action-layer seeder for the browser audit (design.md §D3).
//
// The static fixtures under test-data/audit/ are loaded by
// scripts/run-audit-local.sh through POST /import-legacy; they only ever
// produce tenants, unmatched bank transactions and expenses. This script walks
// the remaining shapes by driving the application's *own* HTTP endpoints —
// confirm, allocate, ignore, revoke, cash receipt and cash void — so every
// state it creates is a state a real user can reach (PRD R7). It never writes
// to the database directly.
//
// Contract for the audit data set (tied to test-data/audit/):
//   * transactions are timestamped 2026-09, descriptions reference 2026-08/09
//   * tenant due days make 2026-09 classify as paid / partial / open / overdue
//     when the run happens after 2026-09-16 (see README "known limits")
//
// Env: AUDIT_BASE, AUDIT_USER, AUDIT_PASS (same names the rest of the harness
// uses). Idempotent: every step skips work that is already in the target state,
// and the write endpoints it calls carry stable idempotency keys.

const BASE = (process.env.AUDIT_BASE || '').replace(/\/+$/, '');
const USER = process.env.AUDIT_USER || '';
const PASS = process.env.AUDIT_PASS || '';

const PERIOD_CURRENT = '2026-09';
const PERIOD_PREV = '2026-08';

// Transaction descriptions are unique per imported row; match on a substring.
const TX = {
  full: 'Rent payment September 2026 flat 3B',
  chenA: 'Rent adjustment for 2026-08 - CHEN ZHIQIANG',
  chenB: 'Additional rent settlement for 2026-08 - CHEN ZHIQIANG',
  zhouPartial: 'Partial rent payment for September 2026 - ZHOU MIN',
  // The ignore demo must sit on a transaction that can never produce a
  // 一键匹配 suggestion — one with no remembered payer relation and no parsed
  // rent month (matching.go returns "unmatched" for the former and only a
  // month-choice "candidate" for the latter). MICHAEL OBRIEN's transaction has
  // both a PAYER-MIKE relation and a parsed 2026-09, so ignoring it suppresses
  // the very suggestion the verification block below asserts. A savings
  // transfer qualifies instead; "ignore this, it is not rent" is also the
  // honest reading of that row.
  ignored: 'Transfer into savings reserve',
  revoke: 'Rounding adjustment credit',
};

const TENANT = {
  liNa: '李娜',
  chen: '陈志强',
  zhou: '周敏',
  wang: '王芳',
  priya: 'Priya Nair',
  michael: "Michael O'Brien",
};

let cookie = '';

function fail(message) {
  console.error(`seed.mjs: ${message}`);
  process.exit(1);
}

function log(message) {
  console.log(`  ${message}`);
}

if (!BASE) fail('AUDIT_BASE is required (e.g. http://127.0.0.1:18090)');
if (!USER || !PASS) fail('AUDIT_USER and AUDIT_PASS are required');

function unescapeHtml(value) {
  return value
    .replace(/&amp;/g, '&')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&#39;/g, "'")
    .replace(/&#34;/g, '"')
    .replace(/&quot;/g, '"');
}

async function request(method, path, form) {
  const headers = {};
  if (form) headers['content-type'] = 'application/x-www-form-urlencoded';
  if (cookie) headers['cookie'] = cookie;
  let response;
  try {
    response = await fetch(BASE + path, {
      method,
      headers,
      body: form ? new URLSearchParams(form).toString() : undefined,
      redirect: 'manual',
    });
  } catch (error) {
    fail(`${method} ${path} failed at the network level: ${error.message}`);
  }
  const setCookies = typeof response.headers.getSetCookie === 'function' ? response.headers.getSetCookie() : [];
  for (const raw of setCookies) {
    const match = /^rentops_session=([^;]*)/.exec(raw);
    if (match) cookie = `rentops_session=${match[1]}`;
  }
  const body = await response.text();
  return { status: response.status, location: response.headers.get('location') || '', body };
}

function expectRedirect(response, path, needle) {
  if (response.status !== 302) {
    fail(`POST ${path} expected 302, got ${response.status}: ${response.body.slice(0, 400)}`);
  }
  if (!response.location.includes(needle)) {
    fail(`POST ${path} expected redirect containing ${needle}, got ${response.location}`);
  }
}

async function get(path) {
  const response = await request('GET', path);
  if (response.status !== 200) fail(`GET ${path} expected 200, got ${response.status}`);
  return response.body;
}

async function post(path, form) {
  return request('POST', path, form);
}

// ---- parsing -------------------------------------------------------------

function parseBillingRows(html) {
  const rows = [];
  // The row carries other attributes too (`id="transaction-row-<key>"`), so match
  // the class anywhere in the opening tag rather than insisting it comes first:
  // attribute order is a rendering detail, not part of this parser's contract.
  const rowPattern = /<tr\b[^>]*\bclass="(income|expense)"[^>]*>([\s\S]*?)<\/tr>/g;
  let match;
  while ((match = rowPattern.exec(html))) {
    const segment = match[2];
    // Only income rows carry a transaction_id (their action forms need it);
    // expense rows have no actions, so keep them keyed by description instead.
    const idMatch = /name="transaction_id" value="(\d+)"/.exec(segment);
    const description = unescapeHtml((/class="description">([\s\S]*?)<\/div>/.exec(segment) || [, ''])[1]).trim();
    const status = (/<span class="status ([a-z_]+)">/.exec(segment) || [, ''])[1];
    rows.push({ id: idMatch ? idMatch[1] : null, direction: match[1], description, status });
  }
  return rows;
}

// The rent-dashboard read model is reachable, in a session that has a DB, only
// through /bills. handleBills (page_data_routes.go:439) delegates to
// renderRentDashboard, which picks its template by request path: /bills and
// /dunning are the only live cases. The legacy no-database fallback template and
// its `rent-row` markup were removed (09-19-legacy-dashboard-template-removal):
// /rent-dashboard never reached that branch once a session existed — the handler
// short-circuits to the room workspace and now fails with 503 when no database
// is configured. So the per-obligation rows to read are /bills' own.
//
// status=all is required to see every state: the page's "未结清" filter is
// open ∪ overdue ∪ partial (dashboard_filters.go:81-90) and hides `paid`.
//
// The mobile card list (collection-bill-card) repeats the same tenants, so
// this deliberately anchors on the desktop table's <tr> shape to avoid
// double-counting.
function parseBillsRows(html) {
  const rows = [];
  const rowPattern = /<tr><td class="mono">[^<]*<\/td><td><a href="\/tenants\/(\d+)[^"]*"><strong>([\s\S]*?)<\/strong><\/a>[\s\S]*?<span class="status ([a-z_]+)">/g;
  let match;
  while ((match = rowPattern.exec(html))) {
    rows.push({ tenantId: match[1], tenant: unescapeHtml(match[2]).trim(), status: match[3] });
  }
  return rows;
}

function parseTenantId(html, name) {
  const rows = html.split('<tr class="tenant-row"').slice(1);
  for (const segment of rows) {
    if (!segment.includes(name) && !unescapeHtml(segment).includes(name)) continue;
    // The detail link carries the list's search term back as ?search=, so the
    // id is not followed by the closing quote. Anchor on the digits alone, the
    // way parseBillingRows' row pattern already does.
    const idMatch = /href="\/tenants\/(\d+)/.exec(segment);
    if (idMatch) return idMatch[1];
  }
  fail(`tenant ${name} was not found on /tenants (did /import-legacy run?)`);
}

// The billing default filter is "all" today but is scheduled to become
// "pending". Fetching each stored status explicitly keeps this script correct
// either way, and the union covers expense rows (which "pending" excludes).
async function fetchBillingRows() {
  const byId = new Map();
  for (const status of ['unmatched', 'partial', 'matched', 'ignored']) {
    const rows = parseBillingRows(await get(`/billing?match_status=${status}&page_size=100`));
    for (const row of rows) byId.set(row.id ?? `desc:${row.description}`, row);
  }
  return [...byId.values()];
}

function findTransaction(rows, needle) {
  const matches = rows.filter((row) => row.description.includes(needle));
  if (matches.length !== 1) {
    fail(`expected exactly one transaction matching "${needle}", found ${matches.length}`);
  }
  return matches[0];
}

function findBillsRow(rows, tenant) {
  const matches = rows.filter((row) => row.tenant.includes(tenant));
  if (matches.length !== 1) fail(`expected exactly one /bills row for ${tenant}, found ${matches.length}`);
  return matches[0];
}

// ---- actions -------------------------------------------------------------

async function ensureRentAllocation(transaction, tenantId, period, amount, { expectStatus, key }) {
  if (transaction.status === expectStatus) {
    log(`skip ${transaction.description} — already ${expectStatus}`);
    return;
  }
  if (transaction.status !== 'unmatched') {
    fail(`${transaction.description} is ${transaction.status}, expected unmatched before allocating`);
  }
  const response = await post('/billing/allocate', {
    transaction_id: transaction.id,
    allocation_kind: 'rent',
    tenant_id: tenantId,
    period,
    amount,
    idempotency_key: key,
  });
  expectRedirect(response, '/billing/allocate', 'message=allocation_saved');
  log(`allocated ${amount} of ${transaction.description} to ${period} -> ${expectStatus}`);
}

async function ensureConfirm(transaction, tenantId, period) {
  if (transaction.status === 'matched') {
    log(`skip ${transaction.description} — already matched`);
    return;
  }
  if (transaction.status !== 'unmatched') {
    fail(`${transaction.description} is ${transaction.status}, expected unmatched before confirming`);
  }
  const response = await post('/billing/confirm', {
    transaction_id: transaction.id,
    tenant_id: tenantId,
    period,
    remember_payer: '1',
  });
  expectRedirect(response, '/billing/confirm', 'message=rent_confirmed');
  log(`confirmed ${transaction.description} against ${period}`);
}

// Import stores payer_id/payer_name_hint on the tenant but does not create a
// tenant_payers relation, and strict auto-matching keys off those relations.
// Adding one is a normal UI action and is what makes the "自动匹配" suggestion
// (the 一键匹配 button) appear for a transaction that matches a tenant exactly.
async function ensurePayerRelation(tenantId, payerName, payerId) {
  const html = await get(`/tenants/${tenantId}`);
  if (html.includes(`ID: ${payerId}`)) {
    log(`skip payer relation ${payerId} — already present`);
    return;
  }
  const response = await post(`/tenants/${tenantId}/payers`, { payer_name: payerName, payer_id: payerId });
  expectRedirect(response, `/tenants/${tenantId}/payers`, 'message=payer_added');
  log(`added payer relation ${payerId} to tenant ${tenantId}`);
}

async function ensureIgnored(transaction) {
  if (transaction.status === 'ignored') {
    log(`skip ${transaction.description} — already ignored`);
    return;
  }
  const response = await post('/billing/ignore', {
    transaction_id: transaction.id,
    reason: 'audit seed: non-rent transfer parked out of the way',
    idempotency_key: 'audit-seed-ignore-transfer',
  });
  expectRedirect(response, '/billing/ignore', 'message=transaction_action_saved');
  log(`ignored ${transaction.description}`);
}

// A revoke is an action, not a stored status: the transaction returns to
// "unmatched" and its allocation is voided. We first try the idempotent revoke
// (a repeat with the same key succeeds without touching anything); only when
// that reports "nothing to revoke" do we create a match and revoke it.
async function ensureRevokedDemo(transaction, tenantId, period, amount) {
  const revokeForm = {
    transaction_id: transaction.id,
    reason: 'audit seed: revoke after confirm',
    idempotency_key: 'audit-seed-revoke-rounding',
  };
  if (transaction.status === 'matched') {
    const response = await post('/billing/revoke', revokeForm);
    expectRedirect(response, '/billing/revoke', 'message=transaction_action_saved');
    log(`revoked ${transaction.description}`);
    return;
  }
  if (transaction.status !== 'unmatched') {
    fail(`${transaction.description} is ${transaction.status}, expected unmatched or matched`);
  }
  const repeat = await post('/billing/revoke', revokeForm);
  if (repeat.status === 302 && repeat.location.includes('message=transaction_action_saved')) {
    log(`skip ${transaction.description} — revoke already recorded`);
    return;
  }
  await ensureRentAllocation(
    { ...transaction, status: 'unmatched' },
    tenantId,
    period,
    amount,
    { expectStatus: 'matched', key: 'audit-seed-allocate-rounding' },
  );
  const response = await post('/billing/revoke', revokeForm);
  expectRedirect(response, '/billing/revoke', 'message=transaction_action_saved');
  log(`confirmed and revoked ${transaction.description} (voided allocation, back to unmatched)`);
}

function cashForm(tenantId, period, amount, key, note, receivedAt) {
  return {
    tenant_id: tenantId,
    period,
    amount,
    currency: 'EUR',
    received_at: receivedAt,
    note,
    idempotency_key: key,
  };
}

async function ensureCashReceipt(tenantId, period, amount, key, note, receivedAt) {
  const form = cashForm(tenantId, period, amount, key, note, receivedAt);
  const preview = await post('/cash-receipts/preview', form);
  if (preview.status === 200) {
    // Fresh create path: preview renders the confirm form.
    const create = await post('/cash-receipts', form);
    expectRedirect(create, '/cash-receipts', 'message=cash_receipt_saved');
    log(`recorded cash receipt ${amount} for tenant ${tenantId} ${period}`);
    return;
  }
  fail(`cash preview for tenant ${tenantId} ${period} expected 200, got ${preview.status}: ${preview.body.slice(0, 300)}`);
}

async function findCashReceiptId(tenantId, period) {
  const html = await get(`/tenants/${tenantId}?from_month=${period}&to_month=${period}`);
  const match = /receipt_id=(\d+)/.exec(html);
  return match ? match[1] : null;
}

async function ensureCashVoid(tenantId, period, amount, key, note, receivedAt) {
  await ensureCashReceipt(tenantId, period, amount, key, note, receivedAt);
  const receiptId = await findCashReceiptId(tenantId, period);
  if (!receiptId) {
    // The receipt is either absent or already voided; voided receipts drop out
    // of the tenant history, so "no void link" means the demo is already done.
    log(`skip cash void for tenant ${tenantId} ${period} — no voidable receipt present`);
    return;
  }
  const preview = await get(`/cash-receipts/void?receipt_id=${receiptId}`);
  if (!preview.includes('确认撤销')) {
    log(`skip cash void for tenant ${tenantId} ${period} — receipt ${receiptId} already voided`);
    return;
  }
  const response = await post('/cash-receipts/void', {
    receipt_id: receiptId,
    reason: 'audit seed: correction demo',
  });
  expectRedirect(response, '/cash-receipts/void', 'message=cash_receipt_voided');
  log(`voided cash receipt ${receiptId} for tenant ${tenantId} ${period}`);
}

// ---- main ----------------------------------------------------------------

async function main() {
  console.log(`==> seeding audit actions at ${BASE} as ${USER}`);

  const login = await post('/login-local', { username: USER, password: PASS });
  if (login.status !== 302 || !login.location.includes('/rent-dashboard')) {
    fail(`login failed: status ${login.status} location ${login.location}`);
  }
  log('logged in');

  // Obligations are generated lazily by page loads; load every month the seed
  // allocates against before touching /billing/allocate or /cash-receipts.
  for (const period of ['2026-07', PERIOD_PREV, PERIOD_CURRENT]) {
    await get(`/rent-dashboard?period=${period}`);
  }
  log('generated rent obligations');

  const tenantsHtml = await get('/tenants');
  const tenantId = {
    liNa: parseTenantId(tenantsHtml, TENANT.liNa),
    chen: parseTenantId(tenantsHtml, TENANT.chen),
    zhou: parseTenantId(tenantsHtml, TENANT.zhou),
    wang: parseTenantId(tenantsHtml, TENANT.wang),
    priya: parseTenantId(tenantsHtml, TENANT.priya),
    michael: parseTenantId(tenantsHtml, TENANT.michael),
  };
  log(`tenant ids: ${JSON.stringify(tenantId)}`);

  // Payer relations from the fixture's payer_id/payer_name_hint columns.
  await ensurePayerRelation(tenantId.liNa, 'LI NA', 'PAYER-LI-NA');
  await ensurePayerRelation(tenantId.chen, 'CHEN ZHIQIANG', 'PAYER-CHEN');
  await ensurePayerRelation(tenantId.zhou, 'ZHOU MIN', 'PAYER-ZHOU');
  await ensurePayerRelation(tenantId.priya, 'PRIYA NAIR', 'PAYER-PRIYA');
  await ensurePayerRelation(tenantId.michael, 'MICHAEL OBRIEN', 'PAYER-MIKE');

  let rows = await fetchBillingRows();
  if (rows.length === 0) fail('/billing returned no rows — did POST /import-legacy run?');

  // matched: full rent confirmation (dashboard "paid" for 李娜 2026-09).
  await ensureConfirm(findTransaction(rows, TX.full), tenantId.liNa, PERIOD_CURRENT);

  // matched: full cross-month allocation (陈志强 2026-08 gets 300/2000).
  await ensureRentAllocation(findTransaction(rows, TX.chenA), tenantId.chen, PERIOD_PREV, '300.00', {
    expectStatus: 'matched',
    key: 'audit-seed-allocate-chen-a',
  });

  // partial transaction: allocate only part of the 400.00 remainder
  // (陈志强 2026-08 obligation reaches 550/2000 -> dashboard partial).
  await ensureRentAllocation(findTransaction(rows, TX.chenB), tenantId.chen, PERIOD_PREV, '250.00', {
    expectStatus: 'partial',
    key: 'audit-seed-allocate-chen-b',
  });

  // matched: full allocation that still leaves the obligation partial
  // (周敏 2026-09 dashboard "partial" branch).
  await ensureRentAllocation(findTransaction(rows, TX.zhouPartial), tenantId.zhou, PERIOD_CURRENT, '1000.00', {
    expectStatus: 'matched',
    key: 'audit-seed-allocate-zhou',
  });

  // Ignored transaction: a non-rent transfer, parked so the "已忽略" state is
  // visible. Deliberately not a payable-looking row — see the TX table.
  await ensureIgnored(findTransaction(rows, TX.ignored));

  // MICHAEL OBRIEN's rent is deliberately left untouched. It is the only row
  // in the fixture that satisfies every strict-match condition at once
  // (remembered payer + parsed month + amount equal to the obligation), so it
  // is what makes the 一键匹配 suggestion render for the assertion below.

  // Revoke flow: confirm then revoke, leaving unmatched + a voided allocation.
  await ensureRevokedDemo(findTransaction(rows, TX.revoke), tenantId.wang, PERIOD_CURRENT, '25.00');

  // Cash receipt kept (陈志强 2026-08 reaches 850/2000) and a second one voided.
  await ensureCashReceipt(tenantId.chen, PERIOD_PREV, '300.00', 'audit-seed-cash-chen', 'audit seed cash kept', '2026-08-15');
  await ensureCashVoid(tenantId.wang, PERIOD_PREV, '200.00', 'audit-seed-cash-wang', 'audit seed cash voided', '2026-08-20');

  // ---- verification: re-read and fail loudly on any missing shape --------
  rows = await fetchBillingRows();
  const expectations = [
    [TX.full, 'matched'],
    [TX.chenA, 'matched'],
    [TX.chenB, 'partial'],
    [TX.zhouPartial, 'matched'],
    [TX.ignored, 'ignored'],
    [TX.revoke, 'unmatched'],
  ];
  for (const [needle, status] of expectations) {
    const row = findTransaction(rows, needle);
    if (row.status !== status) fail(`verification: "${needle}" is ${row.status}, expected ${status}`);
  }
  const expenseRows = rows.filter((row) => row.direction === 'expense');
  if (expenseRows.length < 2) fail(`verification: expected >=2 expense (DEBIT) rows, found ${expenseRows.length}`);
  const unmatched = rows.filter((row) => row.status === 'unmatched');
  if (unmatched.length < 2) fail(`verification: expected >=2 unmatched rows, found ${unmatched.length}`);

  // Auto-match suggestion (PRD §9 "自动匹配"): a known payer whose amount
  // exactly covers one rent obligation renders the one-click confirmation.
  const unmatchedPage = await get('/billing?match_status=unmatched&page_size=100');
  if (!unmatchedPage.includes('一键匹配')) {
    fail('verification: no auto-match (一键匹配) suggestion rendered for an unmatched transaction');
  }

  const currentHtml = await get(`/bills?period=${PERIOD_CURRENT}&status=all&page_size=50`);
  const statuses = new Set(parseBillsRows(currentHtml).map((row) => row.status));
  for (const required of ['paid', 'partial', 'open', 'overdue']) {
    if (!statuses.has(required)) {
      fail(`verification: /bills ${PERIOD_CURRENT} has no "${required}" row (saw ${[...statuses].join(', ') || 'no rows'}; collection-table present: ${currentHtml.includes('collection-table')})`);
    }
  }
  const prevRows = parseBillsRows(await get(`/bills?period=${PERIOD_PREV}&status=all&page_size=50`));
  const chenPrev = findBillsRow(prevRows, TENANT.chen);
  if (chenPrev.status !== 'partial') {
    fail(`verification: /bills ${PERIOD_PREV} 陈志强 expected partial, got ${chenPrev.status}`);
  }

  console.log('==> seed complete: all reachable §D3 branches present');
  console.log('    billing: matched / partial / ignored / unmatched / expense rows / auto-match suggestion');
  console.log('    tenants: payer relations, long CN + long EN names/addresses');
  console.log(`    /bills ${PERIOD_CURRENT}: paid / partial / open / overdue`);
  console.log(`    /bills ${PERIOD_PREV}: partial (mixed bank + cash payments)`);
  console.log('    NOT reachable (documented): billing candidate/needs_review, /bills needs_review');
}

main().catch((error) => fail(error.stack || String(error)));
