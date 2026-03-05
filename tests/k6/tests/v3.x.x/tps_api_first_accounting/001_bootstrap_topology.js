import * as auth from '../../../pkg/auth.js';
import { createTopology, getBenchConfig, parsePositiveInt } from './bench_topology.js';

const bootstrapVUs = parsePositiveInt(__ENV.BOOTSTRAP_VUS, 1);

export const options = {
  scenarios: {
    bootstrap_topology: {
      exec: 'default',
      executor: 'per-vu-iterations',
      iterations: 1,
      vus: bootstrapVUs,
      maxDuration: '15m'
    }
  }
};

export function setup() {
  const token = auth.generateToken();
  const cfg = getBenchConfig();

  console.log(
    `[api-first/bootstrap] namespace=${cfg.namespace} orgs=${cfg.orgCount} ledgers_per_org=${cfg.ledgersPerOrg} accounts_per_type=${cfg.accountsPerType} vus=${bootstrapVUs}`
  );

  return {
    token,
    cfg
  };
}

export default function (data) {
  const topology = createTopology(data.token, data.cfg);

  console.log(
    `[api-first/bootstrap] complete namespace=${topology.namespace} organizations=${topology.orgCount} ledgers=${topology.ledgerCount} accounts=${topology.accountCount}`
  );
}
