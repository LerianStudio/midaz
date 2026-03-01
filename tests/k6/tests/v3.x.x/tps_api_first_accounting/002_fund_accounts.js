import * as auth from '../../../pkg/auth.js';
import { createTopology, fundTopology, getBenchConfig } from './bench_topology.js';

export const options = {
  scenarios: {
    fund_accounts: {
      exec: 'default',
      executor: 'per-vu-iterations',
      iterations: 1,
      vus: 1,
      maxDuration: '20m'
    }
  }
};

export function setup() {
  const token = auth.generateToken();
  const cfg = getBenchConfig();

  console.log(
    `[api-first/fund] namespace=${cfg.namespace} orgs=${cfg.orgCount} ledgers_per_org=${cfg.ledgersPerOrg} accounts_per_type=${cfg.accountsPerType} fund_amount=${cfg.fundAmount}`
  );

  return {
    token,
    cfg
  };
}

export default function (data) {
  const topology = createTopology(data.token, data.cfg);
  fundTopology(data.token, topology, data.cfg);

  console.log(
    `[api-first/fund] complete namespace=${topology.namespace} funded_accounts=${topology.accountCount}`
  );
}
