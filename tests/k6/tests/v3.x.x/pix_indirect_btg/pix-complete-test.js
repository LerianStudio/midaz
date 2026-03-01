/**
 * ============================================================================
 * PIX COMPLETE TEST - Teste Integrado com Fluxo Completo
 * ============================================================================
 *
 * Este teste utiliza o setup completo que cria todas as entidades necessárias
 * (Account, Holder, Alias, DICT) e executa testes de PIX.
 *
 * IDs FIXOS (OBRIGATÓRIOS - NÃO ALTERAR):
 *   MIDAZ_ORGANIZATION_ID = 019be10f-df74-78ce-ac1c-0ef1e8d810fb
 *   MIDAZ_LEDGER_ID       = 019be10f-fa03-77a3-b395-aa8c7974a2c0
 *
 * FLUXO DO SETUP:
 *   1. USAR Organization/Ledger fixos (NÃO criar)
 *   2. CRIAR Asset BRL
 *   3. CRIAR Account (Midaz Onboarding)
 *   4. CRIAR Holder (CRM)
 *   5. CRIAR Alias (CRM)
 *   6. CRIAR DICT Entry (PIX)
 *   7. EXECUTAR testes de PIX
 *
 * VARIÁVEIS DE AMBIENTE:
 *   - ENVIRONMENT: dev, sandbox, vpc (default: dev)
 *   - NUM_ACCOUNTS: Número de contas a criar (default: 5)
 *   - TEST_TYPE: smoke, load, stress (default: smoke)
 *   - LOG: DEBUG, ERROR, OFF (default: OFF)
 *
 * USAGE:
 *   k6 run pix-complete-test.js
 *   k6 run pix-complete-test.js -e NUM_ACCOUNTS=10 -e TEST_TYPE=load
 *   k6 run pix-complete-test.js -e LOG=DEBUG
 *
 * ============================================================================
 */

import { sleep } from 'k6';
import * as pix from '../../../pkg/pix.js';
import * as generators from './lib/generators.js';
import * as scenarios from './lib/scenarios.js';
import * as pixSetup from './setup/pix-complete-setup.js';

// ============================================================================
// CONFIGURAÇÕES
// ============================================================================
const ENVIRONMENT = __ENV.ENVIRONMENT || 'dev';
const NUM_ACCOUNTS = parseInt(__ENV.NUM_ACCOUNTS || '5');
const TEST_TYPE = (__ENV.TEST_TYPE || 'smoke').toLowerCase();
const LOG = (__ENV.LOG || 'OFF').toUpperCase();

// ============================================================================
// CONFIGURAÇÕES POR TIPO DE TESTE
// ============================================================================
const TEST_CONFIG = {
  smoke: {
    vus: 1,
    duration: '1m',
    thresholds: {
      http_req_duration: ['p(95)<3000'],
      http_req_failed: ['rate<0.2']
    }
  },
  load: {
    vus: 5,
    duration: '5m',
    thresholds: {
      http_req_duration: ['p(95)<4000'],
      http_req_failed: ['rate<0.1']
    }
  },
  stress: {
    vus: 20,
    duration: '10m',
    thresholds: {
      http_req_duration: ['p(95)<6000'],
      http_req_failed: ['rate<0.15']
    }
  }
};

const config = TEST_CONFIG[TEST_TYPE] || TEST_CONFIG.smoke;

// ============================================================================
// K6 OPTIONS
// ============================================================================
export const options = {
  scenarios: {
    collection_test: {
      exec: 'collectionScenario',
      executor: 'constant-vus',
      vus: config.vus,
      duration: config.duration,
      startTime: '0s'
    },
    cashout_test: {
      exec: 'cashoutScenario',
      executor: 'constant-vus',
      vus: config.vus,
      duration: config.duration,
      startTime: '15s'
    }
  },
  thresholds: config.thresholds
};

// ============================================================================
// SETUP - Criação de Entidades Completas
// ============================================================================
export function setup() {
  console.log('\n' + '='.repeat(70));
  console.log('     PIX COMPLETE TEST - Teste Integrado');
  console.log('='.repeat(70));
  console.log(`Environment: ${ENVIRONMENT.toUpperCase()}`);
  console.log(`Test Type: ${TEST_TYPE.toUpperCase()}`);
  console.log(`VUs: ${config.vus}`);
  console.log(`Duration: ${config.duration}`);
  console.log(`Accounts to create: ${NUM_ACCOUNTS}`);
  console.log('='.repeat(70) + '\n');

  // Executa o setup completo
  const setupData = pixSetup.defaultSetup(NUM_ACCOUNTS);

  if (!setupData.success) {
    console.error('[SETUP] Falha no setup - verifique os logs');
    console.error(`[SETUP] Erros: ${JSON.stringify(setupData.errors)}`);
  }

  // Resumo das entidades criadas
  console.log('\n[SETUP] Entidades disponíveis para teste:');
  console.log(`  Organization: ${setupData.organizationId} (FIXO)`);
  console.log(`  Ledger: ${setupData.ledgerId} (FIXO)`);
  console.log(`  Accounts: ${setupData.accounts.length}`);
  console.log(`  Holders: ${setupData.holders.length}`);
  console.log(`  Aliases: ${setupData.aliases.length}`);
  console.log(`  DICT Entries: ${setupData.dictEntries.length}`);
  console.log('  PIX Keys:');
  console.log(`    - Email: ${setupData.pixKeys.emailKeys.length}`);
  console.log(`    - Phone: ${setupData.pixKeys.phoneKeys.length}`);
  console.log(`    - CPF: ${setupData.pixKeys.cpfKeys.length}`);
  console.log(`    - CNPJ: ${setupData.pixKeys.cnpjKeys.length}`);
  console.log(`    - EVP: ${setupData.pixKeys.randomKeys.length}`);

  return setupData;
}

// ============================================================================
// CENÁRIO: Collection (Cobrança PIX)
// ============================================================================
export function collectionScenario(data) {
  scenarios.collectionScenario(data, {
    prefix: 'Complete',
    log: LOG,
    includeHolderId: true
  });
}

// ============================================================================
// CENÁRIO: Cashout (Pagamento PIX)
// ============================================================================
export function cashoutScenario(data) {
  scenarios.cashoutScenario(data, {
    prefix: 'Complete',
    log: LOG,
    useTargetAccount: true,
    randomKeyType: 'EVP'
  });
}

// ============================================================================
// TEARDOWN
// ============================================================================
export function teardown(data) {
  const duration = ((Date.now() - data.startTime) / 1000).toFixed(2);

  console.log('\n' + '='.repeat(70));
  console.log('              PIX COMPLETE TEST - TEARDOWN');
  console.log('='.repeat(70));
  console.log(`Duração total: ${duration}s`);
  console.log(`Organization: ${data.organizationId} (FIXO - não criado)`);
  console.log(`Ledger: ${data.ledgerId} (FIXO - não criado)`);
  console.log('-'.repeat(70));
  console.log('ENTIDADES UTILIZADAS:');
  console.log(`  Accounts: ${data.accounts.length}`);
  console.log(`  Holders: ${data.holders.length}`);
  console.log(`  Aliases: ${data.aliases.length}`);
  console.log(`  DICT Entries: ${data.dictEntries.length}`);
  console.log('='.repeat(70) + '\n');
}

// ============================================================================
// HANDLE SUMMARY
// ============================================================================
export function handleSummary(summaryData) {
  const duration = Math.round(summaryData.state.testRunDurationMs / 1000);
  const requests = summaryData.metrics.http_reqs?.values?.count || 0;
  const avgLatency = Math.round(summaryData.metrics.http_req_duration?.values?.avg || 0);
  const p95Latency = Math.round(summaryData.metrics.http_req_duration?.values?.['p(95)'] || 0);
  const failedRate = ((summaryData.metrics.http_req_failed?.values?.rate || 0) * 100).toFixed(2);

  console.log('\n' + '='.repeat(70));
  console.log('           PIX COMPLETE TEST - SUMMARY');
  console.log('='.repeat(70));
  console.log(`Test Type: ${TEST_TYPE.toUpperCase()}`);
  console.log(`Duration: ${duration}s`);
  console.log(`Total Requests: ${requests}`);
  console.log(`Avg Latency: ${avgLatency}ms`);
  console.log(`P95 Latency: ${p95Latency}ms`);
  console.log(`Failed Rate: ${failedRate}%`);
  console.log('-'.repeat(70));
  console.log('IDs UTILIZADOS (FIXOS):');
  console.log(`  Organization: ${pixSetup.MIDAZ_ORGANIZATION_ID}`);
  console.log(`  Ledger: ${pixSetup.MIDAZ_LEDGER_ID}`);
  console.log('='.repeat(70) + '\n');

  return {
    'stdout': '',
    'pix-complete-summary.json': JSON.stringify({
      testType: TEST_TYPE,
      environment: ENVIRONMENT,
      duration: duration,
      requests: requests,
      latency: { avg: avgLatency, p95: p95Latency },
      failedRate: parseFloat(failedRate),
      fixedIds: {
        organizationId: pixSetup.MIDAZ_ORGANIZATION_ID,
        ledgerId: pixSetup.MIDAZ_LEDGER_ID
      }
    }, null, 2)
  };
}
