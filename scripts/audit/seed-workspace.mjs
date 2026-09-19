// Seed the /rent-dashboard workspace with properties, rooms and tenancies.
//
// run-audit-local.sh imports the legacy fixtures (bank-results.jsonl,
// tenants.json, expenses.json) and then runs seed.mjs, which drives the billing
// and tenant branches. Neither of them creates a property or a room: the
// imported tenants exist without a room, so the workspace's three views have
// nothing to show. This script fills that gap through the app's own HTTP
// endpoints (no direct SQL) so the workspace renders real tree rows.
//
// Usage (against a running scripts/run-audit-local.sh instance):
//   AUDIT_BASE=http://127.0.0.1:18093 \
//   AUDIT_USER=<primary account> AUDIT_PASS=<password> \
//     node scripts/audit/seed-workspace.mjs
//
// It is idempotent per run: every call creates fresh properties/rooms, so run
// it once per disposable instance.

const BASE = (process.env.AUDIT_BASE || 'http://127.0.0.1:18093').replace(/\/+$/, '');
const USER = process.env.AUDIT_USER;
const PASS = process.env.AUDIT_PASS;
const PERIOD = process.env.AUDIT_PERIOD || '2026-09';

if (!USER || !PASS) {
  console.error('AUDIT_USER and AUDIT_PASS are required');
  process.exit(1);
}

let cookie = '';

function fail(message) {
  console.error(`FAIL ${message}`);
  process.exit(1);
}

async function request(method, path, form) {
  const headers = {};
  if (cookie) headers.cookie = cookie;
  let body;
  if (form) {
    headers['content-type'] = 'application/x-www-form-urlencoded';
    body = new URLSearchParams(form).toString();
  }
  const response = await fetch(BASE + path, { method, headers, body, redirect: 'manual' });
  const setCookies = typeof response.headers.getSetCookie === 'function'
    ? response.headers.getSetCookie()
    : [response.headers.get('set-cookie')].filter(Boolean);
  if (setCookies.length && !cookie) {
    cookie = setCookies.map((value) => value.split(';')[0]).join('; ');
  }
  const text = await response.text();
  return { status: response.status, location: response.headers.get('location') || '', text };
}

const post = (path, form) => request('POST', path, form);
const get = (path) => request('GET', path);

function expectRedirect(result, label) {
  if (result.status !== 302) {
    fail(`${label}: expected 302, got ${result.status} ${result.text.slice(0, 300)}`);
  }
  if (result.location.includes('error=')) {
    fail(`${label}: ${result.location}`);
  }
  return result.location;
}

function idsFrom(html, pattern) {
  return [...new Set([...html.matchAll(pattern)].map((match) => match[1]))];
}

async function main() {
  console.log(`==> seeding workspace structure at ${BASE}`);

  expectRedirect(await post('/login-local', { username: USER, password: PASS }), 'login');

  // 1. Properties.
  const properties = [
    { name: 'Rosewood Court', city: 'Dublin 8', address: 'Rosewood Court, Dublin 8' },
    { name: 'Kimmage Mews', city: 'Dublin 12', address: 'Kimmage Mews, Dublin 12' },
  ];
  for (const item of properties) {
    expectRedirect(await post('/properties', {
      action: 'save', name: item.name, city_region: item.city, address: item.address,
      timezone: 'Europe/Dublin', notes: 'audit workspace seed',
    }), `create property ${item.name}`);
  }
  const propertyIds = idsFrom((await get('/properties')).text, /\/properties\/(\d+)/g);
  if (propertyIds.length < properties.length) {
    fail(`expected ${properties.length} properties, found ${propertyIds.length}`);
  }
  console.log(`    properties: ${propertyIds.join(', ')}`);

  // 2. Rooms. Two of them stay without a tenancy so the workspace has 空置 rows.
  const roomPlan = [
    { propertyIndex: 0, label: '01', type: '双人间', capacity: 2, rent: '1250.00', dueDay: 1 },
    { propertyIndex: 0, label: '02', type: '双人间', capacity: 2, rent: '1160.00', dueDay: 1 },
    { propertyIndex: 0, label: '03', type: '单人间', capacity: 1, rent: '900.00', dueDay: 5 },
    { propertyIndex: 0, label: '04', type: '单人间', capacity: 1, rent: '820.00', dueDay: 25 },
    { propertyIndex: 1, label: 'A', type: '双人间', capacity: 2, rent: '1400.00', dueDay: 5 },
    { propertyIndex: 1, label: 'B', type: '单人间', capacity: 1, rent: '760.00', dueDay: 20 },
  ];
  for (const room of roomPlan) {
    expectRedirect(await post('/rooms', {
      action: 'save', property_id: propertyIds[room.propertyIndex], room_label: room.label,
      room_type: room.type, capacity: String(room.capacity), monthly_rent: room.rent,
      due_day: String(room.dueDay), active_from: PERIOD, notes: 'audit workspace seed',
    }), `create room ${room.label}`);
  }
  const roomsHtml = (await get('/rooms')).text;
  const roomIds = idsFrom(roomsHtml, /\/rooms\/(\d+)/g);
  console.log(`    rooms: ${roomIds.join(', ')}`);

  // 3. Tenancies: the imported tenants get a room, so their obligations land in
  //    the workspace. Labels are matched back to the room ids by the row order
  //    the /rooms page renders, which is the creation order above.
  const tenantsHtml = (await get('/tenants')).text;
  const tenantIds = idsFrom(tenantsHtml, /\/tenants\/(\d+)/g);
  if (tenantIds.length < 4 || roomIds.length < roomPlan.length) {
    fail(`need at least 4 tenants and ${roomPlan.length} rooms, got ${tenantIds.length}/${roomIds.length}`);
  }
  const plan = [
    { room: 0, tenant: tenantIds[0], rent: '1250.00', dueDay: 1 },
    { room: 1, tenant: tenantIds[1], rent: '1160.00', dueDay: 1 },
    { room: 2, tenant: tenantIds[2], rent: '900.00', dueDay: 5 },
    { room: 4, tenant: tenantIds[3], rent: '1400.00', dueDay: 5 },
    { room: 5, tenant: tenantIds[4] || tenantIds[0], rent: '760.00', dueDay: 20 },
  ];
  for (const item of plan) {
    const roomID = roomIds[item.room];
    const tenantID = item.tenant;
    expectRedirect(await post('/tenancies', {
      room_id: roomID, effective_month: PERIOD, start_date: `${PERIOD}-01`,
      contract_date: `${PERIOD}-01`, move_in_date: `${PERIOD}-01`,
      monthly_rent: item.rent, due_day: String(item.dueDay),
      tenant_ids: [tenantID],
    }), `create tenancy room ${roomID} tenant ${tenantID}`);
  }

  // 4. Load the workspace so the lazy obligation generator runs.
  await get(`/rent-dashboard?period=${PERIOD}`);
  const workspace = (await get(`/rent-dashboard?period=${PERIOD}`)).text;
  const obligationIds = idsFrom(workspace, /name="obligation_id" value="(\d+)"/g);
  console.log(`    obligations visible on the workspace: ${obligationIds.length}`);

  // 5. Settle one obligation so the chips show a non-zero 已缴满 bucket.
  if (obligationIds.length) {
    const location = expectRedirect(await post('/bills/settle', {
      obligation_id: obligationIds[0], period: PERIOD, status: 'all', sort: 'status',
      page: '1', page_size: '12', reason: 'audit workspace seed: 已缴满样本',
    }), 'settle sample obligation');
    console.log(`    settled obligation ${obligationIds[0]} (${location.split('?')[1] || location})`);
  }

  console.log('==> workspace structure seeded');
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
