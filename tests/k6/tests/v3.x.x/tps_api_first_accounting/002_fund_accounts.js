import * as auth from '../../../pkg/auth.js';
import { createTopology, fundTopology, getBenchConfig, parsePositiveInt } from './bench_topology.js';
import exec from 'k6/execution';

const fundVUs = parsePositiveInt(__ENV.FUND_VUS, 10);

export const options = {
  scenarios: {
    fund_accounts: {
      exec: 'default',
      executor: 'per-vu-iterations',
      iterations: 1,
      vus: fundVUs,
      maxDuration: '60m'
    }
  }
};

/**
 * setup() runs once before VUs start. It creates (or rediscovers) the full
 * topology and returns a serialisable object that every VU receives as `data`.
 *
 * We also flatten the account list here so each VU can cheaply slice its share
 * without recomputing anything.
 */
export function setup() {
  const token = auth.generateToken();
  const cfg = getBenchConfig();

  console.log(
    `[api-first/fund] namespace=${cfg.namespace} orgs=${cfg.orgCount} ledgers_per_org=${cfg.ledgersPerOrg} accounts_per_type=${cfg.accountsPerType} fund_amount=${cfg.fundAmount} vus=${fundVUs}`
  );

  // Create (or rediscover) the full topology -- orgs, ledgers, assets, accounts.
  const topology = createTopology(token, cfg);

  // Build a flat list of { organizationId, ledgerId, alias } so VUs can slice it.
  const flatAccounts = [];

  for (const org of topology.organizations) {
    for (const ledger of org.ledgers) {
      for (const alias of ledger.accountAliases) {
        flatAccounts.push({
          organizationId: org.id,
          ledgerId: ledger.id,
          alias
        });
      }
    }
  }

  console.log(
    `[api-first/fund] topology ready total_accounts=${flatAccounts.length} vus=${fundVUs}`
  );

  return {
    token,
    cfg,
    flatAccounts,
    namespace: topology.namespace,
    accountCount: topology.accountCount
  };
}

/**
 * Each VU gets its own slice of accounts to fund.
 *
 * With VU=1..N, we slice the flat list so that:
 *   VU 1 gets indices 0, N, 2N, 3N, ...
 *   VU 2 gets indices 1, N+1, 2N+1, ...
 *   ...etc (round-robin distribution for even load)
 *
 * This avoids contention and ensures every account is funded exactly once.
 */
export default function (data) {
  const vuIndex = exec.vu.idInTest - 1; // 0-based
  const totalVUs = fundVUs;
  const total = data.flatAccounts.length;

  // Compute this VU's share via round-robin.
  const myAccounts = [];

  for (let i = vuIndex; i < total; i += totalVUs) {
    myAccounts.push(data.flatAccounts[i]);
  }

  if (myAccounts.length === 0) {
    console.log(`[api-first/fund] VU ${vuIndex + 1}/${totalVUs} has no accounts to fund, skipping`);
    return;
  }

  console.log(
    `[api-first/fund] VU ${vuIndex + 1}/${totalVUs} funding ${myAccounts.length} of ${total} accounts`
  );

  // Build a minimal topology structure that fundTopology() can consume.
  // Group by orgId -> ledgerId to match the expected shape.
  const orgMap = {};

  for (const entry of myAccounts) {
    if (!orgMap[entry.organizationId]) {
      orgMap[entry.organizationId] = {};
    }

    if (!orgMap[entry.organizationId][entry.ledgerId]) {
      orgMap[entry.organizationId][entry.ledgerId] = [];
    }

    orgMap[entry.organizationId][entry.ledgerId].push(entry.alias);
  }

  // Reconstruct the topology shape that fundTopology expects.
  const slicedTopology = {
    organizations: []
  };

  for (const orgId of Object.keys(orgMap)) {
    const ledgers = [];

    for (const ledgerId of Object.keys(orgMap[orgId])) {
      ledgers.push({
        id: ledgerId,
        accountAliases: orgMap[orgId][ledgerId]
      });
    }

    slicedTopology.organizations.push({
      id: orgId,
      ledgers
    });
  }

  fundTopology(data.token, slicedTopology, data.cfg);

  console.log(
    `[api-first/fund] VU ${vuIndex + 1}/${totalVUs} complete funded=${myAccounts.length}`
  );
}
