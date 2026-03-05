import { check, sleep } from 'k6';
import http from 'k6/http';
import exec from 'k6/execution';
import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';
import * as midaz from '../../../pkg/midaz.js';

const DEFAULT_ACCOUNT_TYPES = ['cacc', 'savings', 'card', 'bsacc', 'ewallet', 'cash'];

const idempotentUpsertStatuses = http.expectedStatuses({ min: 200, max: 399 }, 409);

http.setResponseCallback(idempotentUpsertStatuses);

function defaultBenchNamespace() {
  return `api_bench_${Date.now().toString(36).slice(-8)}`;
}

export function parsePositiveInt(raw, fallback) {
  const parsed = parseInt(raw, 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
}

export function sanitizeNamespace(raw) {
  const base = (raw || 'api_bench').toLowerCase().replace(/[^a-z0-9_]/g, '_').replace(/_+/g, '_');
  const trimmed = base.replace(/^_+|_+$/g, '');
  return (trimmed || 'api_bench').slice(0, 18);
}

export function getBenchConfig() {
  return {
    namespace: sanitizeNamespace(__ENV.BENCH_NAMESPACE || defaultBenchNamespace()),
    orgCount: parsePositiveInt(__ENV.ORG_COUNT, 1),
    ledgersPerOrg: parsePositiveInt(__ENV.LEDGERS_PER_ORG, 1),
    accountsPerType: parsePositiveInt(__ENV.ACCOUNTS_PER_TYPE, 500),
    accountTypes: DEFAULT_ACCOUNT_TYPES,
    fundAmount: __ENV.FUND_AMOUNT || '1000000.00',
    transactionAmount: __ENV.TRANSACTION_AMOUNT || '10.00',
    fundMaxRetries: parsePositiveInt(__ENV.FUND_MAX_RETRIES, 25),
    fundRetrySleepMS: parsePositiveInt(__ENV.FUND_RETRY_SLEEP_MS, 100)
  };
}

function assertStatus(res, label, allowed) {
  const ok = check(res, {
    [`${label} status`]: (r) => allowed.includes(r.status)
  });

  if (!ok) {
    const errorInfo = res.error ? ` error=${res.error}` : '';
    const url = res.request && res.request.url ? ` url=${res.request.url}` : '';
    exec.test.abort(`${label} failed: status=${res.status}${errorInfo}${url} body=${res.body}`);
  }
}

function parseJSON(res, label) {
  try {
    return JSON.parse(res.body);
  } catch (err) {
    exec.test.abort(`${label} response is not valid JSON: ${err}`);
  }
}

function listItems(body) {
  if (!body) {
    return [];
  }

  if (Array.isArray(body)) {
    return body;
  }

  if (body.data && Array.isArray(body.data)) {
    return body.data;
  }

  if (Array.isArray(body.items)) {
    return body.items;
  }

  return [];
}

function organizationName(namespace, orgIndex) {
  return `Lerian Bench ${namespace} Org ${orgIndex + 1}`;
}

function ledgerName(namespace, orgIndex, ledgerIndex) {
  return `ledger_${namespace}_${orgIndex + 1}_${ledgerIndex + 1}`;
}

function accountAlias(namespace, orgIndex, ledgerIndex, accountType, accountIndex) {
  return `@${accountType}_${namespace}_${orgIndex + 1}_${ledgerIndex + 1}_${accountIndex + 1}`;
}

function findOrganizationByName(token, name) {
  const res = midaz.organization.list(token);
  assertStatus(res, 'list organizations', [200]);

  const body = parseJSON(res, 'list organizations');
  const items = listItems(body);

  return items.find((item) => {
    const legalName = item.legalName || item.legal_name || item.name;
    return legalName === name;
  });
}

function findLedgerByName(token, organizationId, name) {
  const res = midaz.ledger.list(token, organizationId);
  assertStatus(res, 'list ledgers', [200]);

  const body = parseJSON(res, 'list ledgers');
  const items = listItems(body);

  return items.find((item) => item.name === name);
}

function createOrganization(token, cfg, orgIndex) {
  const name = organizationName(cfg.namespace, orgIndex);
  const existing = findOrganizationByName(token, name);
  if (existing && existing.id) {
    return existing.id;
  }

  const legalDocument = `${Date.now()}${orgIndex}`.slice(-11);
  const payload = JSON.stringify({
    legalName: name,
    doingBusinessAs: `bench_${cfg.namespace}`,
    legalDocument,
    status: {
      code: 'ACTIVE',
      description: 'Created by API-first k6 benchmark'
    },
    address: {
      line1: 'Avenida Paulista, 1000',
      line2: `Suite ${orgIndex + 1}`,
      zipCode: '01310900',
      city: 'Sao Paulo',
      state: 'SP',
      country: 'BR'
    },
    metadata: {
      scenario: 'tps_api_first_accounting',
      namespace: cfg.namespace,
      orgIndex: orgIndex + 1
    }
  });

  const res = midaz.organization.create(token, payload);
  assertStatus(res, 'create organization', [200, 201]);

  const body = parseJSON(res, 'create organization');
  if (!body || !body.id) {
    exec.test.abort(`create organization did not return id: ${res.body}`);
  }

  return body.id;
}

function ensureLedger(token, organizationId, cfg, orgIndex, ledgerIndex) {
  const name = ledgerName(cfg.namespace, orgIndex, ledgerIndex);
  const existing = findLedgerByName(token, organizationId, name);
  if (existing && existing.id) {
    return existing.id;
  }

  const payload = JSON.stringify({
    name,
    status: {
      code: 'ACTIVE',
      description: 'Created by API-first k6 benchmark'
    },
    metadata: {
      namespace: cfg.namespace,
      orgIndex: orgIndex + 1,
      ledgerIndex: ledgerIndex + 1
    }
  });

  const res = midaz.ledger.create(token, organizationId, payload);
  assertStatus(res, 'create ledger', [200, 201]);

  const body = parseJSON(res, 'create ledger');
  if (!body || !body.id) {
    exec.test.abort(`create ledger did not return id: ${res.body}`);
  }

  return body.id;
}

function ensureAsset(token, organizationId, ledgerId) {
  const payload = JSON.stringify({
    name: 'BRL Asset',
    type: 'currency',
    code: 'BRL',
    status: {
      code: 'ACTIVE',
      description: 'Created by API-first k6 benchmark'
    }
  });

  const res = midaz.asset.create(token, organizationId, ledgerId, payload);
  assertStatus(res, 'create asset BRL', [200, 201, 409]);
}

function ensureAccount(token, organizationId, ledgerId, alias, accountType) {
  const payload = JSON.stringify({
    assetCode: 'BRL',
    name: `Bench ${accountType} ${alias}`,
    alias,
    type: accountType,
    status: {
      code: 'ACTIVE',
      description: 'Created by API-first k6 benchmark'
    }
  });

  const res = midaz.account.create(token, organizationId, ledgerId, payload);
  assertStatus(res, `create account ${alias}`, [200, 201, 409]);
}

export function createTopology(token, cfg) {
  const organizations = [];
  let accountCount = 0;

  for (let orgIndex = 0; orgIndex < cfg.orgCount; orgIndex++) {
    const organizationId = createOrganization(token, cfg, orgIndex);
    const ledgers = [];

    for (let ledgerIndex = 0; ledgerIndex < cfg.ledgersPerOrg; ledgerIndex++) {
      const ledgerId = ensureLedger(token, organizationId, cfg, orgIndex, ledgerIndex);
      ensureAsset(token, organizationId, ledgerId);

      const accountAliases = [];

      for (const accountType of cfg.accountTypes) {
        for (let accountIndex = 0; accountIndex < cfg.accountsPerType; accountIndex++) {
          const alias = accountAlias(cfg.namespace, orgIndex, ledgerIndex, accountType, accountIndex);
          ensureAccount(token, organizationId, ledgerId, alias, accountType);
          accountAliases.push(alias);
          accountCount++;
        }
      }

      ledgers.push({
        id: ledgerId,
        index: ledgerIndex,
        accountAliases
      });
    }

    organizations.push({
      id: organizationId,
      index: orgIndex,
      ledgers
    });
  }

  return {
    namespace: cfg.namespace,
    organizations,
    orgCount: cfg.orgCount,
    ledgerCount: cfg.orgCount * cfg.ledgersPerOrg,
    accountCount
  };
}

function buildFundPayload(alias, amount, namespace) {
  const payload = JSON.stringify({
    description: `Initial funding for ${alias}`,
    send: {
      asset: 'BRL',
      value: amount,
      distribute: {
        to: [
          {
            accountAlias: alias,
            amount: {
              asset: 'BRL',
              value: amount
            },
            metadata: {
              namespace,
              kind: 'initial_funding'
            }
          }
        ]
      }
    },
    metadata: {
      namespace,
      kind: 'initial_funding'
    }
  });

  return payload;
}

function isRetryableFundingStatus(status) {
  return status === 429 || status === 500 || status === 502 || status === 503 || status === 504;
}

function isRetryableFundingResponse(res) {
  if (!res) {
    return false;
  }

  if (isRetryableFundingStatus(res.status)) {
    return true;
  }

  if (res.status !== 422) {
    return false;
  }

  // During setup, account activation can lag a few hundred ms behind create-account.
  // Keep retries scoped to the known transient ineligibility error.
  return typeof res.body === 'string' &&
    res.body.includes('"code":"0019"') &&
    res.body.includes('Account Ineligibility Error');
}

function fundAccountWithRetry(token, organizationId, ledgerId, alias, cfg) {
  const payload = buildFundPayload(alias, cfg.fundAmount, cfg.namespace);
  const idempotencyKey = `${cfg.namespace}-fund-${alias}-v1`;
  let lastResponse = null;

  for (let attempt = 1; attempt <= cfg.fundMaxRetries; attempt++) {
    const requestId = uuidv4();
    const res = midaz.transaction.inflow(token, organizationId, ledgerId, payload, idempotencyKey, requestId);
    lastResponse = res;

    // 200/201 = funded successfully, 409 = idempotency key already used (already funded).
    if (res.status === 200 || res.status === 201 || res.status === 409) {
      return;
    }

    if (!isRetryableFundingResponse(res) || attempt === cfg.fundMaxRetries) {
      assertStatus(res, `fund account ${alias}`, [200, 201, 409]);
      return;
    }

    const backoffSeconds = (cfg.fundRetrySleepMS * attempt) / 1000;
    sleep(backoffSeconds);
  }

  if (lastResponse) {
    assertStatus(lastResponse, `fund account ${alias}`, [200, 201, 409]);
  }
}

export function fundTopology(token, topology, cfg) {
  for (const org of topology.organizations) {
    for (const ledger of org.ledgers) {
      for (const alias of ledger.accountAliases) {
        fundAccountWithRetry(token, org.id, ledger.id, alias, cfg);
      }
    }
  }
}

export function flattenLedgers(topology) {
  const out = [];

  for (const org of topology.organizations) {
    for (const ledger of org.ledgers) {
      out.push({
        organizationId: org.id,
        ledgerId: ledger.id,
        accountAliases: ledger.accountAliases
      });
    }
  }

  return out;
}

export function pickDistinctPair(accountAliases) {
  if (!Array.isArray(accountAliases) || accountAliases.length < 2) {
    exec.test.abort('at least two account aliases are required per ledger for transfer scenarios');
  }

  const fromIndex = Math.floor(Math.random() * accountAliases.length);
  let toIndex = Math.floor(Math.random() * accountAliases.length);

  while (toIndex === fromIndex) {
    toIndex = Math.floor(Math.random() * accountAliases.length);
  }

  return {
    fromAlias: accountAliases[fromIndex],
    toAlias: accountAliases[toIndex]
  };
}
