// k6 load test — oversell prevention (task.md Scenario 2).
//
// A single product is stocked with STOCK units, then BUYERS distinct customers
// each try to buy one unit at the same time. The guarded atomic decrement in the
// order repository must let exactly STOCK orders through (HTTP 201) and reject the
// remaining BUYERS-STOCK with insufficient stock (HTTP 409) — never oversell.
//
// Why distinct buyers: order idempotency is server-derived from sha256(userID,
// cart) and reserved in Redis, so the same user resubmitting the same cart
// collapses to one order. Distinct buyers give BUYERS independent attempts and
// faithfully model "many customers racing for the last items".
//
// Run (server must be listening on BASE_URL with WORKER_GATEWAY_FAILURE_RATE=0):
//   docker run --rm --network host -v "$PWD/scripts":/scripts \
//     grafana/k6 run /scripts/load_test.js
//
// Tunable via env: BASE_URL, STOCK, BUYERS, PASSWORD.

import http from 'k6/http';
import { check } from 'k6';
import { Counter } from 'k6/metrics';

const BASE = __ENV.BASE_URL || 'http://localhost:8080/api/v1';
const STOCK = Number(__ENV.STOCK || 100);
const BUYERS = Number(__ENV.BUYERS || 120);
const PASSWORD = __ENV.PASSWORD || 'password123';

const JSON_HEADERS = { 'Content-Type': 'application/json' };

const placed = new Counter('orders_placed');             // 201 Created
const stockRejected = new Counter('orders_stock_rejected'); // 409 + "insufficient stock"
const unexpected = new Counter('orders_unexpected');     // anything else

export const options = {
  scenarios: {
    buy: {
      executor: 'per-vu-iterations',
      vus: BUYERS,
      iterations: 1,
      maxDuration: '60s',
    },
  },
  // The whole point of the test: the split must be exact.
  thresholds: {
    orders_placed: [`count==${STOCK}`],
    orders_stock_rejected: [`count==${BUYERS - STOCK}`],
    orders_unexpected: ['count==0'],
  },
  setupTimeout: '180s',
};

function authHeaders(token) {
  return { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` };
}

// register creates a user (optionally admin) and returns their bearer token.
function register(email, name, role) {
  const body = { name, email, password: PASSWORD };
  if (role) body.role = role;

  const created = http.post(`${BASE}/users`, JSON.stringify(body), { headers: JSON_HEADERS });
  if (created.status !== 201) {
    throw new Error(`create user ${email} failed: HTTP ${created.status} ${created.body}`);
  }
  const login = http.post(`${BASE}/auth/login`, JSON.stringify({ email, password: PASSWORD }), {
    headers: JSON_HEADERS,
  });
  if (login.status !== 200) {
    throw new Error(`login ${email} failed: HTTP ${login.status} ${login.body}`);
  }
  const token = login.json('access_token');
  if (!token) throw new Error(`login ${email} returned no access_token: ${login.body}`);
  return token;
}

export function setup() {
  const run = Date.now(); // unique per run -> no email/product collisions on re-runs

  // 1. Admin, used only to create the product.
  const adminToken = register(`admin_${run}@test.local`, 'load-admin', 'admin');

  // 2. Product stocked with exactly STOCK units.
  const prodRes = http.post(
    `${BASE}/products`,
    JSON.stringify({ name: `load-${run}`, price: 9.99, quantity: STOCK }),
    { headers: authHeaders(adminToken) },
  );
  if (prodRes.status !== 201) {
    throw new Error(`create product failed: HTTP ${prodRes.status} ${prodRes.body}`);
  }
  const productID = prodRes.json('id');
  const quantity = prodRes.json('quantity');
  if (quantity !== STOCK) {
    throw new Error(`product seeded with quantity=${quantity}, expected ${STOCK}`);
  }

  // 3. BUYERS distinct customers, each with their own token.
  const tokens = [];
  for (let i = 0; i < BUYERS; i++) {
    tokens.push(register(`buyer_${run}_${i}@test.local`, `buyer-${i}`, null));
  }

  console.log(`setup complete: productID=${productID}, stock=${STOCK}, buyers=${tokens.length}`);
  return { productID, tokens };
}

export default function (data) {
  const token = data.tokens[__VU - 1]; // one buyer per VU, each used exactly once
  const res = http.post(
    `${BASE}/orders`,
    JSON.stringify({ items: [{ product_id: data.productID, quantity: 1 }] }),
    { headers: authHeaders(token) },
  );

  if (res.status === 201) {
    placed.add(1);
  } else if (res.status === 409 && res.body.includes('insufficient stock')) {
    stockRejected.add(1);
  } else {
    unexpected.add(1);
    console.error(`unexpected order response: HTTP ${res.status} ${res.body}`);
  }

  check(res, {
    'placed (201) or stock-rejected (409)': (r) =>
      r.status === 201 || (r.status === 409 && r.body.includes('insufficient stock')),
  });
}
