// Seed a disposable RentOps workspace through the same forms a landlord uses.
// The fixture intentionally covers a shared room, a partial payment, an empty
// room, and a future plan without writing test-only database rows.

const BASE = (process.env.AUDIT_BASE || 'http://127.0.0.1:18090').replace(/\/+$/, '');
const USER = process.env.AUDIT_USER || '';
const PASS = process.env.AUDIT_PASS || '';
const RUN_ID = process.env.AUDIT_RUN_ID || `audit-${Date.now()}`;

if (!USER || !PASS) {
  console.error('AUDIT_USER and AUDIT_PASS are required');
  process.exit(1);
}

let cookie = '';

function fail(message) {
  throw new Error(message);
}

function dublinMonth(offset = 0) {
  const parts = new Intl.DateTimeFormat('en', {
    timeZone: 'Europe/Dublin', year: 'numeric', month: '2-digit',
  }).formatToParts(new Date());
  const year = Number(parts.find((part) => part.type === 'year').value);
  const month = Number(parts.find((part) => part.type === 'month').value);
  const value = year * 12 + month - 1 + offset;
  return `${Math.floor(value / 12)}-${String((value % 12) + 1).padStart(2, '0')}`;
}

function dublinToday() {
  const parts = new Intl.DateTimeFormat('en', {
    timeZone: 'Europe/Dublin', year: 'numeric', month: '2-digit', day: '2-digit',
  }).formatToParts(new Date());
  return `${parts.find((part) => part.type === 'year').value}-${parts.find((part) => part.type === 'month').value}-${parts.find((part) => part.type === 'day').value}`;
}

function encodeForm(form) {
  const values = new URLSearchParams();
  for (const [key, value] of Object.entries(form)) {
    if (Array.isArray(value)) value.forEach((item) => values.append(key, String(item)));
    else values.set(key, String(value));
  }
  return values.toString();
}

async function request(method, path, form) {
  const headers = {};
  if (cookie) headers.cookie = cookie;
  if (form) headers['content-type'] = 'application/x-www-form-urlencoded';
  const response = await fetch(BASE + path, {
    method, headers, body: form ? encodeForm(form) : undefined, redirect: 'manual',
  });
  const setCookies = typeof response.headers.getSetCookie === 'function'
    ? response.headers.getSetCookie()
    : [response.headers.get('set-cookie')].filter(Boolean);
  if (setCookies.length) {
    cookie = setCookies.map((value) => value.split(';')[0]).join('; ');
  }
  return {
    status: response.status,
    location: response.headers.get('location') || '',
    text: await response.text(),
  };
}

function expectRedirect(result, label, contains = '') {
  if (result.status !== 302 || result.location.includes('error=') || (contains && !result.location.includes(contains))) {
    fail(`${label}: expected redirect ${contains || '(success)'}, got ${result.status} ${result.location} ${result.text.slice(0, 240)}`);
  }
}

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function linkedIDForRow(html, label, pathPrefix) {
  const hrefPattern = new RegExp(`href="${escapeRegExp(pathPrefix)}/([1-9][0-9]*)`, 'g');
  let searchFrom = 0;
  let candidate = '';
  let index;
  while ((index = html.indexOf(label, searchFrom)) >= 0) {
    const start = html.lastIndexOf('<tr', index);
    const close = html.indexOf('</tr>', index);
    const fragment = start >= 0 && close > index
      ? html.slice(start, close)
      : html.slice(Math.max(0, index - 3000), Math.min(html.length, index + 3000));
    const matches = [...fragment.matchAll(hrefPattern)];
    if (matches.length) candidate = matches[matches.length - 1][1];
    searchFrom = index + label.length;
  }
  if (!candidate) fail(`detail link for ${label} is missing`);
  return candidate;
}

async function createTenant(tenant) {
  const result = await request('POST', '/tenants', {
    name: tenant.name, display_alias: tenant.alias, email: tenant.email,
    payer_id: tenant.payerID, payer_name_hint: tenant.payerName, status: 'active',
  });
  expectRedirect(result, `create tenant ${tenant.name}`, 'tenant_added');
  const html = (await request('GET', '/tenants')).text;
  return linkedIDForRow(html, tenant.name, '/tenants');
}

async function createRoom(room, propertyID, period) {
  const result = await request('POST', '/rooms', {
    action: 'save', property_id: propertyID, room_label: room.label,
    room_type: room.type, capacity: room.capacity, notes: `${RUN_ID} browser audit`, period,
    effective_month: period, monthly_rent: '1000.00', due_day: '5',
  });
  expectRedirect(result, `create room ${room.label}`, 'room_saved');
  const html = (await request('GET', `/rooms?period=${period}&property_id=${propertyID}`)).text;
  return linkedIDForRow(html, room.label, '/rooms');
}

async function savePlan(roomID, period, rent, dueDay, members) {
  const editor = await request('GET', `/rooms/${roomID}?period=${period}&rent=1`);
  if (editor.status !== 200) fail(`rent-plan editor for room ${roomID} returned ${editor.status}`);
  const versionMatch = editor.text.match(/name="plan_version" value="([0-9]+)"/);
  if (!versionMatch) fail(`rent-plan timeline version for room ${roomID} is missing`);
  const result = await request('POST', `/rooms/${roomID}/rent-plan`, {
    action: 'save', period, plan_version: versionMatch[1], effective_month: period,
    monthly_rent: rent, due_day: dueDay,
    tenant_id: members.map((member) => member.id),
    responsibility: members.map((member) => member.amount),
  });
  expectRedirect(result, `save room ${roomID} rent plan`, 'rent_plan_saved');
}

async function recordCash(tenantID, period, amount, payerTenantID, key) {
  const form = {
    tenant_id: tenantID, payer_tenant_id: payerTenantID, payer_name: '', period,
    amount, currency: 'EUR', received_at: dublinToday(),
    idempotency_key: `${RUN_ID}-${key}`, note: `${RUN_ID} browser audit receipt`,
  };
  const preview = await request('POST', '/cash-receipts/preview', form);
  if (preview.status !== 200 || !preview.text.includes('确认现金入账')) {
    fail(`cash receipt preview failed: ${preview.status} ${preview.text.slice(0, 300)}`);
  }
  const result = await request('POST', '/cash-receipts', form);
  expectRedirect(result, `record ${key} cash receipt`, 'cash_receipt_saved');
}

async function main() {
  const period = dublinMonth();
  const future = dublinMonth(1);
  const day = Number(dublinToday().slice(-2));
  const overdueDueDay = Math.max(1, day - 1);

  console.log(`==> seeding room-rent workspace at ${BASE} for ${period}`);
  expectRedirect(await request('POST', '/login-local', { username: USER, password: PASS }), 'login');

  const propertySpecs = [
    { name: `${RUN_ID} Rosewood Court`, city: 'Dublin 8', address: 'Rosewood Court, Dublin 8' },
    { name: `${RUN_ID} Kimmage Mews`, city: 'Dublin 12', address: 'Kimmage Mews, Dublin 12' },
  ];
  const propertyIDs = [];
  for (const property of propertySpecs) {
    expectRedirect(await request('POST', '/properties', {
      action: 'save', name: property.name, city_region: property.city,
      address: property.address, timezone: 'Europe/Dublin', notes: `${RUN_ID} browser audit`,
    }), `create property ${property.name}`, 'property_saved');
    const html = (await request('GET', '/properties')).text;
    propertyIDs.push(linkedIDForRow(html, property.name, '/properties'));
  }

  const people = [
    { name: `${RUN_ID} Aoife Murphy`, alias: 'Aoife', email: 'aoife@example.test', payerID: `${RUN_ID}-payer-a`, payerName: 'Aoife Murphy' },
    { name: `${RUN_ID} Brian Doyle`, alias: 'Brian', email: 'brian@example.test', payerID: `${RUN_ID}-payer-b`, payerName: 'Brian Doyle' },
    { name: `${RUN_ID} Chen Xi`, alias: 'Chen', email: 'chen@example.test', payerID: `${RUN_ID}-payer-c`, payerName: 'Chen Xi' },
    { name: `${RUN_ID} Dara Wu`, alias: 'Dara', email: 'dara@example.test', payerID: `${RUN_ID}-payer-d`, payerName: 'Dara Wu' },
  ];
  const tenantIDs = [];
  for (const person of people) tenantIDs.push(await createTenant(person));

  const roomSpecs = [
    { property: 0, label: '01', type: '双人间', capacity: 2 },
    { property: 0, label: '02', type: '单人间', capacity: 1 },
    { property: 0, label: '03', type: '空置房间', capacity: 1 },
    { property: 1, label: 'A', type: '未来入住', capacity: 1 },
  ];
  const roomIDs = [];
  for (const room of roomSpecs) roomIDs.push(await createRoom(room, propertyIDs[room.property], period));

  await savePlan(roomIDs[0], period, '1200.00', 5, [
    { id: tenantIDs[0], amount: '700.00' },
    { id: tenantIDs[1], amount: '500.00' },
  ]);
  await savePlan(roomIDs[1], period, '900.00', overdueDueDay, [
    { id: tenantIDs[2], amount: '900.00' },
  ]);
  await savePlan(roomIDs[3], future, '760.00', 20, [
    { id: tenantIDs[3], amount: '760.00' },
  ]);

  // Cash is used only for the browser fixture: one shared-room responsibility is
  // paid by its housemate, while the second room stays partially outstanding.
  await recordCash(tenantIDs[0], period, '700.00', tenantIDs[0], 'aoife');
  await recordCash(tenantIDs[1], period, '500.00', tenantIDs[0], 'brian-paid-by-aoife');
  await recordCash(tenantIDs[2], period, '100.00', tenantIDs[2], 'chen-partial');

  const roomPage = await request('GET', `/rooms/${roomIDs[0]}?period=${period}`);
  if (roomPage.status !== 200) {
    fail(`room detail returned ${roomPage.status}`);
  }
  const tenantDetail = await request('GET', `/tenants/${tenantIDs[1]}?period=${period}`);
  if (tenantDetail.status !== 200 || !tenantDetail.text.includes(`/rooms/${roomIDs[0]}`)) {
    fail('tenant detail did not link its room-level responsibility back to the room');
  }
  const futureWorkspace = await request('GET', `/rent-dashboard?view=rooms&period=${future}`);
  if (futureWorkspace.status !== 200 || !futureWorkspace.text.includes('预计')) {
    fail('future rent-plan preview was not visible on the workspace');
  }
  for (const method of ['GET', 'POST']) {
    const response = await request(method, '/tenancies', method === 'POST' ? {} : undefined);
    if (response.status !== 404) fail(`/tenancies ${method} returned ${response.status}, expected 404`);
  }

  console.log(`    properties: ${propertyIDs.join(', ')}`);
  console.log(`    rooms: ${roomIDs.join(', ')}`);
  console.log(`    tenants: ${tenantIDs.join(', ')}`);
  console.log(`    current period: ${period}; future preview: ${future}`);
  console.log('==> room-rent workspace seeded');
}

main().catch((error) => {
  console.error(`seed-workspace.mjs: ${error.message}`);
  process.exit(1);
});
