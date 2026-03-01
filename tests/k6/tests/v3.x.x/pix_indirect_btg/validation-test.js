/**
 * PIX Validation Test - Teste com Setup Dinâmico ou IDs Existentes
 *
 * Este teste valida o comportamento do PIX. Pode usar setup dinâmico
 * (criando Account, Holder, Alias, DICT) ou IDs existentes.
 *
 * Environment Variables:
 *   - ENVIRONMENT: dev, sandbox, vpc (default: dev)
 *   - ACCOUNT_ID: ID da conta existente (opcional - se não fornecido, usa setup dinâmico)
 *   - RECEIVER_KEY: Chave PIX do recebedor (opcional)
 *   - LOG: DEBUG, ERROR, OFF (default: OFF)
 *   - K6_THINK_TIME_MODE: fast, realistic, stress (default: fast)
 *
 * Usage:
 *   # Com setup dinâmico (recomendado)
 *   k6 run validation-test.js -e ENVIRONMENT=sandbox -e K6_ABORT_ON_ERROR=false
 *
 *   # Usando conta específica existente
 *   k6 run validation-test.js -e ACCOUNT_ID=550e8400-e29b-41d4-a716-446655440001
 *
 *   # Com chave PIX específica
 *   k6 run validation-test.js -e ACCOUNT_ID=<id> -e RECEIVER_KEY=user@example.com
 *
 *   # Com debug
 *   k6 run validation-test.js -e LOG=DEBUG
 */

import * as auth from '../../../pkg/auth.js';
import * as pix from '../../../pkg/pix.js';
import * as generators from './lib/generators.js';
import * as pixSetup from './setup/pix-complete-setup.js';

// Configuration
const ENVIRONMENT = __ENV.ENVIRONMENT || 'dev';
const ACCOUNT_ID = __ENV.ACCOUNT_ID;
const RECEIVER_KEY = __ENV.RECEIVER_KEY;
const LOG = (__ENV.LOG || 'OFF').toUpperCase();
const USE_DYNAMIC_SETUP = !ACCOUNT_ID;

export const options = {
  scenarios: {
    validation: {
      exec: 'validationTest',
      executor: 'shared-iterations',
      vus: 1,
      iterations: 1,
      maxDuration: '2m'
    }
  },
  thresholds: {
    http_req_duration: ['p(95)<2000'],
    http_req_failed: ['rate<0.5'] // Tolerant for validation
  }
};

export function setup() {
  console.log('\n' + '='.repeat(70));
  console.log('         PIX VALIDATION TEST');
  console.log('='.repeat(70));
  console.log(`Environment: ${ENVIRONMENT.toUpperCase()}`);
  console.log(`Log Level: ${LOG}`);

  if (USE_DYNAMIC_SETUP) {
    console.log('Mode: Dynamic Setup (creating Account, Holder, Alias, DICT)');
    console.log('='.repeat(70));

    // Execute complete dynamic setup
    const setupData = pixSetup.defaultSetup(1); // Only 1 account needed for validation

    if (!setupData.success || setupData.accounts.length === 0) {
      console.error('[SETUP] Failed to create test resources');
      console.error(`[SETUP] Errors: ${JSON.stringify(setupData.errors)}`);
      throw new Error('Setup failed: Could not create account');
    }

    const account = setupData.accounts[0];
    const emailKey = setupData.pixKeys.emailKeys[0];

    console.log(`Account ID: ${account.id}`);
    console.log(`Receiver Key: ${emailKey?.key || account.document}`);
    console.log('='.repeat(70) + '\n');

    return {
      token: setupData.token,
      accountId: account.id,
      receiverKey: emailKey?.key || account.document
    };
  } else {
    console.log('Mode: Existing Account');
    console.log(`Account ID: ${ACCOUNT_ID}`);
    console.log(`Receiver Key: ${RECEIVER_KEY || '(gerado dinamicamente)'}`);
    console.log('='.repeat(70));

    const token = auth.generateToken();

    console.log('='.repeat(70) + '\n');

    return {
      token,
      accountId: ACCOUNT_ID,
      receiverKey: RECEIVER_KEY
    };
  }
}

/**
 * Teste de validação completo
 * Executa: Collection Create -> Get -> List -> Delete
 */
export function validationTest(data) {
  console.log('\n[STEP 1] Criando Collection PIX...');

  const txId = generators.generateTxId(32);
  const idempotencyKey = generators.generateIdempotencyKey();
  const receiverKey = data.receiverKey || generators.generateEmailKey();

  // Payload mínimo - apenas campos essenciais
  const createPayload = JSON.stringify({
    txId: txId,
    receiverKey: receiverKey,
    amount: generators.generateAmount(10, 100),
    expirationSeconds: 3600,
    description: `Validation Test - ${new Date().toISOString()}`
  });

  if (LOG === 'DEBUG') {
    console.log(`Payload: ${createPayload}`);
  }

  const createStartTime = Date.now();
  const createRes = pix.collection.create(data.token, data.accountId, createPayload, idempotencyKey);
  const createDuration = Date.now() - createStartTime;

  console.log(`[CREATE] Status: ${createRes.status}, Duration: ${createDuration}ms`);

  if (createRes.status !== 201 && createRes.status !== 200) {
    console.error(`[ERROR] Falha ao criar collection: ${createRes.body}`);
    return {
      success: false,
      step: 'create',
      status: createRes.status,
      error: createRes.body
    };
  }

  let collectionId;
  try {
    const body = JSON.parse(createRes.body);
    collectionId = body.id;
    console.log(`[SUCCESS] Collection criada: ${collectionId}`);
    console.log(`          TxID: ${txId}`);
    console.log(`          Status: ${body.status}`);
  } catch (e) {
    console.error(`[ERROR] Falha ao parsear resposta: ${e.message}`);
    return { success: false, step: 'create-parse', error: e.message };
  }

  // Step 2: Get Collection by ID
  console.log('\n[STEP 2] Recuperando Collection por ID...');

  const getStartTime = Date.now();
  const getRes = pix.collection.getById(data.token, data.accountId, collectionId);
  const getDuration = Date.now() - getStartTime;

  console.log(`[GET] Status: ${getRes.status}, Duration: ${getDuration}ms`);

  if (getRes.status !== 200) {
    console.error(`[ERROR] Falha ao recuperar collection: ${getRes.body}`);
  } else {
    try {
      const getBody = JSON.parse(getRes.body);
      console.log(`[SUCCESS] Collection recuperada`);
      console.log(`          ID: ${getBody.id}`);
      console.log(`          Status: ${getBody.status}`);
      console.log(`          Amount: ${getBody.amount}`);
    } catch (e) {
      console.error(`[ERROR] Falha ao parsear resposta GET: ${e.message}`);
    }
  }

  // Step 3: List Collections
  console.log('\n[STEP 3] Listando Collections...');

  const listStartTime = Date.now();
  const listRes = pix.collection.list(data.token, data.accountId, 'limit=5');
  const listDuration = Date.now() - listStartTime;

  console.log(`[LIST] Status: ${listRes.status}, Duration: ${listDuration}ms`);

  if (listRes.status === 200) {
    try {
      const listBody = JSON.parse(listRes.body);
      console.log(`[SUCCESS] Total collections: ${listBody.items?.length || 0}`);
    } catch (e) {
      console.log(`[SUCCESS] Lista retornada (parsing skipped)`);
    }
  } else {
    console.error(`[ERROR] Falha ao listar collections: ${listRes.body}`);
  }

  // Step 4: Delete Collection (cleanup)
  console.log('\n[STEP 4] Deletando Collection (cleanup)...');

  const deleteStartTime = Date.now();
  const deleteRes = pix.collection.remove(data.token, data.accountId, collectionId, 'DELETED_BY_USER');
  const deleteDuration = Date.now() - deleteStartTime;

  console.log(`[DELETE] Status: ${deleteRes.status}, Duration: ${deleteDuration}ms`);

  if (deleteRes.status === 200 || deleteRes.status === 204) {
    console.log(`[SUCCESS] Collection deletada`);
  } else {
    console.warn(`[WARNING] Não foi possível deletar (pode já estar processada): ${deleteRes.status}`);
  }

  // Summary
  console.log('\n' + '-'.repeat(70));
  console.log('RESUMO DA VALIDAÇÃO:');
  console.log(`  Create: ${createRes.status === 201 || createRes.status === 200 ? 'OK' : 'FALHA'} (${createDuration}ms)`);
  console.log(`  Get:    ${getRes.status === 200 ? 'OK' : 'FALHA'} (${getDuration}ms)`);
  console.log(`  List:   ${listRes.status === 200 ? 'OK' : 'FALHA'} (${listDuration}ms)`);
  console.log(`  Delete: ${deleteRes.status === 200 || deleteRes.status === 204 ? 'OK' : 'SKIP'} (${deleteDuration}ms)`);
  console.log('-'.repeat(70) + '\n');

  return {
    success: (createRes.status === 201 || createRes.status === 200) &&
             getRes.status === 200 &&
             listRes.status === 200,
    collectionId,
    txId,
    durations: {
      create: createDuration,
      get: getDuration,
      list: listDuration,
      delete: deleteDuration
    },
    steps: {
      create: createRes.status === 201 || createRes.status === 200,
      get: getRes.status === 200,
      list: listRes.status === 200,
      delete: deleteRes.status === 200 || deleteRes.status === 204
    }
  };
}

export function handleSummary(data) {
  const duration = Math.round(data.state.testRunDurationMs / 1000);
  const requests = data.metrics.http_reqs?.values?.count || 0;
  const avgLatency = Math.round(data.metrics.http_req_duration?.values?.avg || 0);
  const failedRate = ((data.metrics.http_req_failed?.values?.rate || 0) * 100).toFixed(2);

  console.log('\n' + '='.repeat(70));
  console.log('                    VALIDATION TEST COMPLETE');
  console.log('='.repeat(70));
  console.log(`Duration: ${duration}s`);
  console.log(`Total Requests: ${requests}`);
  console.log(`Avg Latency: ${avgLatency}ms`);
  console.log(`Failed Rate: ${failedRate}%`);
  console.log('='.repeat(70) + '\n');

  return {};
}
