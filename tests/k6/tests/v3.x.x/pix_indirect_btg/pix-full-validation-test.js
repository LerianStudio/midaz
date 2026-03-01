/**
 * ============================================================================
 * PIX FULL VALIDATION TEST - Testes com IDs Fixos Obrigatórios
 * ============================================================================
 *
 * OBJETIVO:
 * Validar comportamento completo do PIX usando Organization e Ledger já existentes,
 * seguindo o fluxo correto: Account → PIX Operations
 *
 * IDs FIXOS (OBRIGATÓRIOS - NÃO ALTERAR):
 *   MIDAZ_ORGANIZATION_ID = 019be10f-df74-78ce-ac1c-0ef1e8d810fb
 *   MIDAZ_LEDGER_ID       = 019be10f-fa03-77a3-b395-aa8c7974a2c0
 *
 * RESTRIÇÕES:
 *   ❌ NÃO criar Organization
 *   ❌ NÃO criar Ledger
 *   ❌ NÃO sobrescrever ou mockar IDs
 *   ✅ USAR exclusivamente os IDs fornecidos
 *
 * FLUXO EXECUTADO:
 *   1. Verificar/Criar Asset BRL (vinculado ao Org/Ledger fixo)
 *   2. Criar Account (vinculada ao Org/Ledger fixo)
 *   3. Executar testes PIX:
 *      - Collection (Cobrança Imediata)
 *      - Transfer/Cashout (Pagamento)
 *      - Refund (Reembolso)
 *
 * VARIÁVEIS DE AMBIENTE:
 *   - ENVIRONMENT: dev, sandbox, vpc (default: dev)
 *   - LOG: DEBUG, ERROR, OFF (default: OFF)
 *   - K6_ABORT_ON_ERROR: true/false (default: false)
 *   - TEST_SCENARIO: all, collection, cashout, refund (default: all)
 *
 * USAGE:
 *   k6 run pix-full-validation-test.js
 *   k6 run pix-full-validation-test.js -e LOG=DEBUG
 *   k6 run pix-full-validation-test.js -e TEST_SCENARIO=collection
 *
 * ============================================================================
 */

import { sleep } from 'k6';
import * as auth from '../../../pkg/auth.js';
import * as midaz from '../../../pkg/midaz.js';
import * as pix from '../../../pkg/pix.js';
import * as crm from '../../../pkg/crm.js';
import * as generators from './lib/generators.js';

// ============================================================================
// IDs FIXOS OBRIGATÓRIOS - NÃO ALTERAR
// ============================================================================
const MIDAZ_ORGANIZATION_ID = __ENV.MIDAZ_ORGANIZATION_ID || '019be10f-df74-78ce-ac1c-0ef1e8d810fb';
const MIDAZ_LEDGER_ID = __ENV.MIDAZ_LEDGER_ID || '019be10f-fa03-77a3-b395-aa8c7974a2c0';

// ============================================================================
// CONFIGURAÇÕES
// ============================================================================
const ENVIRONMENT = __ENV.ENVIRONMENT || 'dev';
const LOG = (__ENV.LOG || 'OFF').toUpperCase();
const TEST_SCENARIO = (__ENV.TEST_SCENARIO || 'all').toLowerCase();

// ============================================================================
// K6 OPTIONS
// ============================================================================
export const options = {
  scenarios: {
    pix_validation: {
      exec: 'pixValidationTest',
      executor: 'shared-iterations',
      vus: 1,
      iterations: 1,
      maxDuration: '5m'
    }
  },
  thresholds: {
    http_req_duration: ['p(95)<3000'],
    http_req_failed: ['rate<0.5']
  }
};

function parseJsonResponse(res, operationName) {
  try {
    return JSON.parse(res.body);
  } catch (error) {
    logWarn(`${operationName}: resposta JSON inválida (${error.message})`);
    return null;
  }
}

function formatErrorDetails(res, operationName) {
  const body = parseJsonResponse(res, operationName);
  const details = body?.message || body?.error || body?.code;
  return details ? `${res.status} - ${details}` : `${res.status}`;
}

function maskSensitive(value, prefix = 2, suffix = 2) {
  if (typeof value !== 'string' || value.length <= prefix + suffix) {
    return '***';
  }

  return `${value.slice(0, prefix)}***${value.slice(-suffix)}`;
}

// ============================================================================
// SETUP - Configuração Inicial (Executado uma vez)
// ============================================================================
export function setup() {
  const token = auth.generateToken();

  printHeader();
  logInfo(`Environment: ${ENVIRONMENT.toUpperCase()}`);
  logInfo(`Organization ID: ${MIDAZ_ORGANIZATION_ID}`);
  logInfo(`Ledger ID: ${MIDAZ_LEDGER_ID}`);
  logInfo(`Test Scenario: ${TEST_SCENARIO.toUpperCase()}`);
  logInfo(`Log Level: ${LOG}`);
  printSeparator();

  // -------------------------------------------------------------------------
  // STEP 1: Verificar/Criar Asset BRL
  // -------------------------------------------------------------------------
  logStep(1, 'Verificando/Criando Asset BRL');

  const assetPayload = JSON.stringify({
    name: 'Brazilian Real',
    type: 'currency',
    code: 'BRL',
    status: {
      code: 'ACTIVE',
      description: 'Asset created for PIX validation tests'
    }
  });

  const assetRes = midaz.asset.create(token, MIDAZ_ORGANIZATION_ID, MIDAZ_LEDGER_ID, assetPayload);

  if (assetRes.status === 201 || assetRes.status === 200) {
    const assetBody = parseJsonResponse(assetRes, 'Asset BRL create');
    logSuccess(`Asset BRL criado/existente: ${assetBody?.id || 'N/A'}`);
  } else if (assetRes.status === 409) {
    logInfo('Asset BRL já existe (409 Conflict) - OK');
  } else {
    logWarn(`Asset response: ${formatErrorDetails(assetRes, 'Asset BRL create')}`);
  }

  // -------------------------------------------------------------------------
  // STEP 2: Criar Account para testes
  // -------------------------------------------------------------------------
  logStep(2, 'Criando Account para testes PIX');

  const timestamp = Date.now();
  const accountAlias = `@pix_test_${timestamp}_BRL`;

  const accountPayload = JSON.stringify({
    assetCode: 'BRL',
    name: `PIX Validation Account ${timestamp}`,
    alias: accountAlias,
    type: 'deposit',
    status: {
      code: 'ACTIVE',
      description: 'Account for PIX validation tests'
    },
    metadata: {
      testId: `pix-validation-${timestamp}`,
      environment: ENVIRONMENT,
      createdAt: new Date().toISOString()
    }
  });

  const accountRes = midaz.account.create(token, MIDAZ_ORGANIZATION_ID, MIDAZ_LEDGER_ID, accountPayload);

  let accountId = null;
  if (accountRes.status === 201 || accountRes.status === 200) {
    const accountBody = parseJsonResponse(accountRes, 'Account create');
    accountId = accountBody?.id || null;
    if (!accountId) {
      throw new Error('Setup failed: Account create returned invalid JSON payload');
    }

    logSuccess(`Account criada: ${accountId}`);
    logInfo(`  Alias: ${accountAlias}`);
  } else if (accountRes.status === 0) {
    // Serviço não disponível - fail fast
    logError(`Serviço Onboarding não disponível (connection refused)`);
    throw new Error('Setup failed: Onboarding service unavailable');
  } else {
    logError(`Falha ao criar account: ${formatErrorDetails(accountRes, 'Account create')}`);
    throw new Error(`Setup failed: Cannot create account`);
  }

  // -------------------------------------------------------------------------
  // STEP 3: Criar External Account (@external/BRL) para carga de saldo
  // -------------------------------------------------------------------------
  logStep(3, 'Criando External Account (@external/BRL)');

  const externalAccountPayload = JSON.stringify({
    assetCode: 'BRL',
    name: 'External BRL Account',
    alias: '@external/BRL',
    type: 'external',
    status: {
      code: 'ACTIVE',
      description: 'External account for fund transfers'
    }
  });

  const externalAccountRes = midaz.account.create(token, MIDAZ_ORGANIZATION_ID, MIDAZ_LEDGER_ID, externalAccountPayload);

  if (externalAccountRes.status === 201 || externalAccountRes.status === 200) {
    logSuccess('External Account @external/BRL criada');
  } else if (externalAccountRes.status === 409) {
    logInfo('External Account @external/BRL já existe - OK');
  } else {
    logWarn(`External Account response: ${formatErrorDetails(externalAccountRes, 'External account create')}`);
  }

  // -------------------------------------------------------------------------
  // STEP 4: Criar Balance na conta (key: "default")
  // -------------------------------------------------------------------------
  logStep(4, 'Criando Balance na conta');

  const balancePayload = JSON.stringify({
    key: 'default'
  });

  const balanceRes = midaz.balance.create(token, MIDAZ_ORGANIZATION_ID, MIDAZ_LEDGER_ID, accountId, balancePayload);

  if (balanceRes.status === 201 || balanceRes.status === 200) {
    logSuccess('Balance "default" criado');
  } else if (balanceRes.status === 409) {
    logInfo('Balance "default" já existe - OK');
  } else {
    logWarn(`Balance response: ${formatErrorDetails(balanceRes, 'Balance create')}`);
  }

  // -------------------------------------------------------------------------
  // STEP 5: Carregar saldo na conta (R$ 10.000,00)
  // -------------------------------------------------------------------------
  logStep(5, 'Carregando saldo na conta (R$ 10.000,00)');

  const chargeIdempotencyKey = generators.generateIdempotencyKey();
  const chargeRequestId = generators.generateUUID();
  const chargeAmount = '1000000'; // R$ 10.000,00 em centavos

  const chargePayload = JSON.stringify({
    send: {
      asset: 'BRL',
      value: chargeAmount,
      source: {
        from: [
          {
            accountAlias: '@external/BRL',
            amount: {
              asset: 'BRL',
              value: chargeAmount
            },
            metadata: {
              type: 'charge',
              description: 'Initial balance charge for PIX tests'
            }
          }
        ]
      },
      distribute: {
        to: [
          {
            accountAlias: accountAlias,
            balanceKey: 'default',
            amount: {
              asset: 'BRL',
              value: chargeAmount
            },
            metadata: {
              type: 'charge',
              description: 'Initial balance charge for PIX tests'
            }
          }
        ]
      }
    },
    metadata: {
      type: 'charge',
      source: 'k6_pix_validation_test'
    }
  });

  const chargeRes = midaz.transaction.create(token, MIDAZ_ORGANIZATION_ID, MIDAZ_LEDGER_ID, chargePayload, chargeIdempotencyKey, chargeRequestId);

  if (chargeRes.status === 201 || chargeRes.status === 200) {
    logSuccess('Saldo carregado: R$ 10.000,00');
  } else {
    logError(`Falha ao carregar saldo: ${formatErrorDetails(chargeRes, 'Balance charge')}`);
    logWarn('⚠️  ATENÇÃO: Testes de cashout podem falhar por saldo insuficiente!');
  }

  // -------------------------------------------------------------------------
  // STEP 6: Criar Holder A no CRM (Natural Person)
  // -------------------------------------------------------------------------
  logStep(6, 'Criando Holder A no CRM');

  const holderDocument = generators.generateCPF();
  const holderExternalId = `EXT-A-${timestamp}`;
  const holderEmail = `holder.a.${timestamp}@test.com`;
  const holderPhone = generators.generatePhoneKey();

  const holderPayload = JSON.stringify({
    name: 'Jose Maria Test',
    document: holderDocument,
    type: 'NATURAL_PERSON',
    externalId: holderExternalId,
    contact: {
      mobilePhone: holderPhone,
      primaryEmail: holderEmail
    },
    naturalPerson: {
      birthDate: '1990-01-15',
      civilStatus: 'Single',
      gender: 'Male',
      nationality: 'Brazilian',
      favoriteName: 'Holder A',
      socialName: 'Jose Maria Test',
      motherName: 'Maria Test',
      fatherName: 'Jose Test',
      status: 'Active'
    },
    addresses: {
      primary: {
        line1: 'Avenida Paulista, 1000',
        line2: 'Suite 101',
        zipCode: '01310-100',
        city: 'Sao Paulo',
        state: 'SP',
        country: 'BR'
      }
    },
    metadata: {
      source: 'k6_validation_test',
      role: 'sender'
    }
  });

  const holderRes = crm.holder.create(token, MIDAZ_ORGANIZATION_ID, holderPayload);

  let holderId = null;
  if (holderRes.status === 201 || holderRes.status === 200) {
    const holderBody = parseJsonResponse(holderRes, 'Holder create');
    holderId = holderBody?.id || null;
    logSuccess(`Holder A criado: ${holderId}`);
    logInfo(`  Document: ${maskSensitive(holderDocument, 3, 2)}`);
    logInfo(`  Type: NATURAL_PERSON`);
  } else if (holderRes.status === 409) {
    logInfo('Holder A já existe (409 Conflict) - OK');
  } else {
    logError(`Falha ao criar Holder A: ${formatErrorDetails(holderRes, 'Holder create')}`);
    logWarn('⚠️  ATENÇÃO: CRM pode não estar disponível!');
  }

  // -------------------------------------------------------------------------
  // STEP 7: Criar Alias A no CRM (Banking Details)
  // -------------------------------------------------------------------------
  logStep(7, 'Criando Alias A no CRM');

  let pixAccountId = null;
  const aliasAccountNumber = String(Math.floor(Math.random() * 99999999)).padStart(8, '0');

  if (holderId) {
    const aliasPayload = JSON.stringify({
      accountId: accountId,
      ledgerId: MIDAZ_LEDGER_ID,
      bankingDetails: {
        account: aliasAccountNumber,
        bankId: '13866572',
        branch: '0001',
        countryCode: 'BR',
        iban: 'ME59406160025300106274',
        openingDate: new Date().toISOString().split('T')[0],
        type: 'TRAN'
      },
      metadata: {
        source: 'k6_validation_test',
        role: 'sender'
      }
    });

    const aliasRes = crm.alias.create(token, MIDAZ_ORGANIZATION_ID, holderId, aliasPayload);

    if (aliasRes.status === 201 || aliasRes.status === 200) {
      const aliasBody = parseJsonResponse(aliasRes, 'Alias create');
      pixAccountId = aliasBody?.accountId || accountId;
      logSuccess(`Alias A criado: ${aliasBody?.id || 'N/A'}`);
      logInfo(`  PIX Account ID: ${pixAccountId}`);
      logInfo(`  Account Number: ${aliasAccountNumber}`);
    } else if (aliasRes.status === 409) {
      logInfo('Alias A já existe (409 Conflict) - usando accountId como fallback');
      pixAccountId = accountId;
    } else {
      logError(`Falha ao criar Alias A: ${formatErrorDetails(aliasRes, 'Alias create')}`);
      logWarn('⚠️  Usando accountId como fallback para PIX operations');
      pixAccountId = accountId;
    }
  } else {
    logWarn('Holder não criado - usando accountId como fallback para PIX operations');
    pixAccountId = accountId;
  }

  // -------------------------------------------------------------------------
  // STEP 8: Criar PIX Key A (DICT Entry - CPF)
  // -------------------------------------------------------------------------
  logStep(8, 'Criando PIX Key A (DICT Entry)');

  let pixKeyId = null;
  let pixKeyValue = holderDocument;

  if (pixAccountId) {
    const pixKeyPayload = JSON.stringify({
      key: holderDocument,
      keyType: 'CPF'
    });

    const pixKeyRes = pix.dict.create(token, pixAccountId, pixKeyPayload, 'USER_REQUESTED');

    if (pixKeyRes.status === 201 || pixKeyRes.status === 200) {
      const pixKeyBody = parseJsonResponse(pixKeyRes, 'PIX key create');
      pixKeyId = pixKeyBody?.id || null;
      pixKeyValue = pixKeyBody?.keyValue || holderDocument;
      logSuccess(`PIX Key A criada: ${pixKeyId}`);
      logInfo(`  Key Value: ${maskSensitive(pixKeyValue, 4, 2)}`);
      logInfo(`  Key Type: CPF`);

      // Optional endpoint coverage: DICT claim
      const claimPayload = JSON.stringify({
        metadata: {
          source: 'k6_validation_test',
          reason: 'endpoint_coverage'
        }
      });
      const claimRes = pix.dict.claim(token, pixAccountId, pixKeyId, claimPayload, 'PIX_KEY_PORTABILITY');
      if ([200, 201, 202, 400, 404, 409, 422].includes(claimRes.status)) {
        logInfo(`DICT claim endpoint exercised: status=${claimRes.status}`);
      } else {
        logWarn(`DICT claim unexpected status: ${claimRes.status}`);
      }
    } else if (pixKeyRes.status === 409) {
      logInfo('PIX Key A já existe (409 Conflict) - OK');
    } else {
      logError(`Falha ao criar PIX Key A: ${formatErrorDetails(pixKeyRes, 'PIX key create')}`);
    }
  }

  printSeparator();
  logInfo('SETUP COMPLETO - Entidades criadas:');
  logInfo(`  Organization: ${MIDAZ_ORGANIZATION_ID} (pré-existente)`);
  logInfo(`  Ledger: ${MIDAZ_LEDGER_ID} (pré-existente)`);
  logInfo(`  External Account: @external/BRL`);
  logInfo(`  Account: ${accountId} (criada)`);
  logInfo(`  Balance: default (R$ 10.000,00)`);
  logInfo(`  Holder: ${holderId || 'N/A'}`);
  logInfo(`  Alias: PIX Account ID = ${pixAccountId || 'N/A'}`);
  logInfo(`  PIX Key: ${pixKeyValue} (CPF)`);
  printSeparator();

  return {
    token,
    organizationId: MIDAZ_ORGANIZATION_ID,
    ledgerId: MIDAZ_LEDGER_ID,
    accountId,
    accountAlias,
    holderId,
    pixAccountId: pixAccountId || accountId,
    pixKeyId,
    pixKeyValue,
    holderDocument,
    startTime: Date.now()
  };
}

// ============================================================================
// TESTE PRINCIPAL
// ============================================================================
export function pixValidationTest(data) {
  const results = {
    collection: null,
    cashout: null,
    refund: null
  };

  // -------------------------------------------------------------------------
  // TEST 1: PIX Collection (Cobrança Imediata)
  // -------------------------------------------------------------------------
  if (TEST_SCENARIO === 'all' || TEST_SCENARIO === 'collection') {
    results.collection = testPixCollection(data);
  }

  // -------------------------------------------------------------------------
  // TEST 2: PIX Cashout/Transfer (Pagamento)
  // -------------------------------------------------------------------------
  if (TEST_SCENARIO === 'all' || TEST_SCENARIO === 'cashout') {
    results.cashout = testPixCashout(data);
  }

  // -------------------------------------------------------------------------
  // TEST 3: PIX Refund (Reembolso) - Requer transferência prévia
  // -------------------------------------------------------------------------
  if (TEST_SCENARIO === 'all' || TEST_SCENARIO === 'refund') {
    if (results.cashout?.transferId) {
      results.refund = testPixRefund(data, results.cashout.transferId);
    } else {
      logWarn('Refund test skipped: No valid transfer available');
    }
  }

  // -------------------------------------------------------------------------
  // SUMÁRIO FINAL
  // -------------------------------------------------------------------------
  printFinalSummary(results);

  return results;
}

// ============================================================================
// TEST: PIX Collection (Cobrança Imediata)
// ============================================================================
function testPixCollection(data) {
  printTestHeader('PIX COLLECTION - Cobrança Imediata');

  const result = {
    success: false,
    collectionId: null,
    txId: null,
    steps: {}
  };

  // Step 1: Criar Collection
  logStep(1, 'Criar Cobrança PIX');

  const txId = generators.generateTxId(32);
  const idempotencyKey = generators.generateIdempotencyKey();
  const receiverKey = generators.generateEmailKey();
  const amount = generators.generateAmount(10, 100);
  const debtorDocument = generators.generateCPF();
  const debtorName = `Debtor Test ${Date.now()}`;

  const createPayload = JSON.stringify({
    txId: txId,
    receiverKey: receiverKey,
    amount: amount,
    expirationSeconds: 3600,
    debtorDocument: debtorDocument,
    debtorName: debtorName,
    description: `PIX Validation Test - ${new Date().toISOString()}`,
    metadata: {
      source: 'k6_validation_test',
      test: 'immediate_collection',
      testId: `validation-${Date.now()}`,
      organizationId: MIDAZ_ORGANIZATION_ID,
      ledgerId: MIDAZ_LEDGER_ID
    }
  });

  if (LOG === 'DEBUG') {
    logDebug(`Payload: ${createPayload}`);
  }

  const createStart = Date.now();
  const createRes = pix.collection.create(data.token, data.pixAccountId, createPayload, idempotencyKey);
  const createDuration = Date.now() - createStart;

  result.steps.create = {
    status: createRes.status,
    duration: createDuration,
    success: createRes.status === 201 || createRes.status === 200
  };

  if (result.steps.create.success) {
    const body = parseJsonResponse(createRes, 'Collection create');
    if (!body || !body.id) {
      logError('Falha ao interpretar resposta da criação de collection');
      return result;
    }

    result.collectionId = body.id;
    result.txId = txId;
    logSuccess(`Collection criada: ${body.id} (${createDuration}ms)`);
    logInfo(`  TxID: ${txId}`);
    logInfo(`  Receiver Key: ${maskSensitive(receiverKey, 4, 2)}`);
    logInfo(`  Amount: R$ ${amount}`);
  } else {
    logError(`Falha ao criar collection: ${formatErrorDetails(createRes, 'Collection create')}`);
    return result;
  }

  // Step 2: Consultar Collection por ID
  logStep(2, 'Consultar Cobrança por ID');

  sleep(0.5);
  const getStart = Date.now();
  const getRes = pix.collection.getById(data.token, data.pixAccountId, result.collectionId);
  const getDuration = Date.now() - getStart;

  result.steps.getById = {
    status: getRes.status,
    duration: getDuration,
    success: getRes.status === 200
  };

  if (result.steps.getById.success) {
    const body = parseJsonResponse(getRes, 'Collection getById');
    if (!body || body.id !== result.collectionId) {
      result.steps.getById.success = false;
      logError('Inconsistência: Collection getById retornou ID diferente do criado');
      return result;
    }

    logSuccess(`Collection recuperada: status=${body?.status || 'UNKNOWN'} (${getDuration}ms)`);
  } else {
    logError(`Falha ao recuperar collection: ${getRes.status}`);
  }

  // Step 3: Consultar Collection por TxID
  logStep(3, 'Consultar Cobrança por TxID');

  const getByTxIdStart = Date.now();
  const getByTxIdRes = pix.collection.getByTxId(data.token, data.pixAccountId, txId);
  const getByTxIdDuration = Date.now() - getByTxIdStart;

  result.steps.getByTxId = {
    status: getByTxIdRes.status,
    duration: getByTxIdDuration,
    success: getByTxIdRes.status === 200
  };

  if (result.steps.getByTxId.success) {
    const body = parseJsonResponse(getByTxIdRes, 'Collection getByTxId');
    const returnedTxId = body?.txId || body?.transactionIdentification || body?.items?.[0]?.txId;
    if (returnedTxId && returnedTxId !== txId) {
      result.steps.getByTxId.success = false;
      logError('Inconsistência: Collection getByTxId retornou txId diferente do criado');
      return result;
    }

    logSuccess(`Collection por TxID recuperada (${getByTxIdDuration}ms)`);
  } else {
    logError(`Falha ao recuperar por TxID: ${getByTxIdRes.status}`);
  }

  // Step 4: Listar Collections
  logStep(4, 'Listar Cobranças');

  const listStart = Date.now();
  const listRes = pix.collection.list(data.token, data.pixAccountId, 'limit=5');
  const listDuration = Date.now() - listStart;

  result.steps.list = {
    status: listRes.status,
    duration: listDuration,
    success: listRes.status === 200
  };

  if (result.steps.list.success) {
    logSuccess(`Collections listadas (${listDuration}ms)`);
  } else {
    logError(`Falha ao listar: ${listRes.status}`);
  }

  // Step 5: Deletar Collection (cleanup)
  logStep(5, 'Deletar Cobrança (cleanup)');

  const deleteStart = Date.now();
  const deleteRes = pix.collection.remove(data.token, data.pixAccountId, result.collectionId, 'DELETED_BY_USER');
  const deleteDuration = Date.now() - deleteStart;

  result.steps.delete = {
    status: deleteRes.status,
    duration: deleteDuration,
    success: deleteRes.status === 200 || deleteRes.status === 204
  };

  if (result.steps.delete.success) {
    logSuccess(`Collection deletada (${deleteDuration}ms)`);
  } else {
    logWarn(`Não foi possível deletar (${deleteRes.status}) - pode já estar processada`);
  }

  result.success = result.steps.create.success && result.steps.getById.success;
  printTestResult('COLLECTION', result.success, result.steps);

  return result;
}

// ============================================================================
// TEST: PIX Cashout/Transfer (Pagamento)
// ============================================================================
function testPixCashout(data) {
  printTestHeader('PIX CASHOUT - Transferência/Pagamento');

  const result = {
    success: false,
    transferId: null,
    endToEndId: null,
    steps: {}
  };

  // Step 1: Iniciar Pagamento por Chave PIX
  logStep(1, 'Iniciar Pagamento (Initiate)');

  const idempotencyKey = generators.generateIdempotencyKey();
  const pixKey = generators.generateEmailKey();

  // Payload follows Postman collection specification (no description on initiate)
  const initiatePayload = JSON.stringify({
    initiationType: 'KEY',
    key: pixKey
  });

  if (LOG === 'DEBUG') {
    logDebug(`Payload: ${initiatePayload}`);
  }

  const initiateStart = Date.now();
  const initiateRes = pix.transfer.initiate(data.token, data.pixAccountId, initiatePayload, idempotencyKey);
  const initiateDuration = Date.now() - initiateStart;

  result.steps.initiate = {
    status: initiateRes.status,
    duration: initiateDuration,
    success: initiateRes.status === 201 || initiateRes.status === 200
  };

  if (result.steps.initiate.success) {
    const body = parseJsonResponse(initiateRes, 'Transfer initiate');
    if (!body || !body.id) {
      logError('Falha ao interpretar resposta de initiate');
      printTestResult('CASHOUT', false, result.steps);
      return result;
    }

    result.transferId = body.id;
    result.endToEndId = body.endToEndId;
    logSuccess(`Pagamento iniciado: ${body.id} (${initiateDuration}ms)`);
    logInfo(`  PIX Key: ${maskSensitive(pixKey, 4, 2)}`);
    logInfo(`  EndToEndId: ${body.endToEndId || 'pending'}`);
  } else {
    logError(`Falha ao iniciar pagamento: ${formatErrorDetails(initiateRes, 'Transfer initiate')}`);
    printTestResult('CASHOUT', false, result.steps);
    return result;
  }

  // Step 2: Processar Pagamento (dentro de 5 minutos)
  logStep(2, 'Processar Pagamento (Process)');

  sleep(1); // Simula tempo de confirmação do usuário

  const processIdempotency = generators.generateIdempotencyKey();
  const amount = generators.generateAmount(10, 50);

  const processPayload = JSON.stringify({
    initiationId: result.transferId,
    amount: amount,
    description: `K6 Cashout Process - VU ${__VU}`,
    metadata: {
      source: 'k6_validation_test',
      test: 'cashout_transfer',
      vu: __VU
    }
  });

  const processStart = Date.now();
  const processRes = pix.transfer.process(data.token, data.pixAccountId, processPayload, processIdempotency);
  const processDuration = Date.now() - processStart;

  result.steps.process = {
    status: processRes.status,
    duration: processDuration,
    success: processRes.status === 200 || processRes.status === 201 || processRes.status === 202
  };

  if (result.steps.process.success) {
    const body = parseJsonResponse(processRes, 'Transfer process');
    logSuccess(`Pagamento processado: status=${body?.status || 'UNKNOWN'} (${processDuration}ms)`);
    logInfo(`  Amount: R$ ${amount}`);
  } else {
    logError(`Falha ao processar pagamento: ${formatErrorDetails(processRes, 'Transfer process')}`);
  }

  // Step 3: Consultar Transferência
  logStep(3, 'Consultar Transferência');

  const getStart = Date.now();
  const getRes = pix.transfer.getById(data.token, data.pixAccountId, result.transferId);
  const getDuration = Date.now() - getStart;

  result.steps.get = {
    status: getRes.status,
    duration: getDuration,
    success: getRes.status === 200
  };

  if (result.steps.get.success) {
    const body = parseJsonResponse(getRes, 'Transfer getById');
    logSuccess(`Transferência recuperada: status=${body?.status || 'UNKNOWN'} (${getDuration}ms)`);
  } else {
    logWarn(`Não foi possível recuperar transferência: ${getRes.status}`);
  }

  // Step 4: Listar Transferências
  logStep(4, 'Listar Transferências');

  const listStart = Date.now();
  const listRes = pix.transfer.list(data.token, data.pixAccountId, 'limit=5');
  const listDuration = Date.now() - listStart;

  result.steps.list = {
    status: listRes.status,
    duration: listDuration,
    success: listRes.status === 200
  };

  if (result.steps.list.success) {
    logSuccess(`Transferências listadas (${listDuration}ms)`);
  } else {
    logWarn(`Não foi possível listar transferências: ${listRes.status}`);
  }

  // Step 5: Exercitar endpoint de desbloqueio
  logStep(5, 'Tentar desbloquear transferência');

  const unblockStart = Date.now();
  const unblockRes = pix.transfer.unblock(data.token, data.pixAccountId, result.transferId);
  const unblockDuration = Date.now() - unblockStart;

  // Unblock can fail with business status depending on current transfer state.
  // We still count 4xx as covered endpoint execution.
  result.steps.unblock = {
    status: unblockRes.status,
    duration: unblockDuration,
    success: unblockRes.status >= 200 && unblockRes.status < 500
  };

  if (result.steps.unblock.success) {
    logSuccess(`Endpoint unblock exercitado (${unblockDuration}ms, status=${unblockRes.status})`);
  } else {
    logWarn(`Falha inesperada no unblock: ${unblockRes.status}`);
  }

  result.success =
    result.steps.initiate.success &&
    result.steps.process.success &&
    result.steps.get.success;
  printTestResult('CASHOUT', result.success, result.steps);

  return result;
}

// ============================================================================
// TEST: PIX Refund (Reembolso)
// ============================================================================
function testPixRefund(data, transferId) {
  printTestHeader('PIX REFUND - Reembolso');

  const result = {
    success: false,
    refundId: null,
    steps: {}
  };

  // Step 1: Criar Refund
  logStep(1, 'Criar Reembolso');

  const idempotencyKey = generators.generateIdempotencyKey();
  const reasonCode = 'MD06'; // Customer requested refund

  const refundPayload = JSON.stringify({
    amount: '10.00',
    description: `PIX Refund Validation - ${new Date().toISOString()}`
  });

  if (LOG === 'DEBUG') {
    logDebug(`TransferId: ${transferId}`);
    logDebug(`Payload: ${refundPayload}`);
    logDebug(`Reason Code: ${reasonCode}`);
  }

  const createStart = Date.now();
  const createRes = pix.refund.create(data.token, data.pixAccountId, transferId, refundPayload, idempotencyKey, reasonCode);
  const createDuration = Date.now() - createStart;

  result.steps.create = {
    status: createRes.status,
    duration: createDuration,
    success: createRes.status === 201 || createRes.status === 200 || createRes.status === 202
  };

  if (result.steps.create.success) {
    const body = parseJsonResponse(createRes, 'Refund create');
    result.refundId = body?.id || null;
    logSuccess(`Refund criado: ${result.refundId || 'N/A'} (${createDuration}ms)`);
    logInfo(`  Reason Code: ${reasonCode}`);
  } else {
    logWarn(`Refund não processado: ${formatErrorDetails(createRes, 'Refund create')}`);
    logInfo('(Isso pode ser esperado se a transferência não foi completada)');
  }

  // Step 2: Consultar Refund
  if (result.refundId) {
    logStep(2, 'Consultar Reembolso');

    const getStart = Date.now();
    const getRes = pix.refund.getById(data.token, data.pixAccountId, transferId, result.refundId);
    const getDuration = Date.now() - getStart;

    result.steps.get = {
      status: getRes.status,
      duration: getDuration,
      success: getRes.status === 200
    };

    if (result.steps.get.success) {
      logSuccess(`Refund recuperado (${getDuration}ms)`);
    } else {
      logWarn(`Não foi possível recuperar refund: ${getRes.status}`);
    }
  }

  // Step 3: Listar Refunds
  logStep(3, 'Listar Reembolsos');

  const listStart = Date.now();
  const listRes = pix.refund.list(data.token, data.pixAccountId, transferId, 'limit=5');
  const listDuration = Date.now() - listStart;

  result.steps.list = {
    status: listRes.status,
    duration: listDuration,
    success: listRes.status === 200
  };

  if (result.steps.list.success) {
    logSuccess(`Refunds listados (${listDuration}ms)`);
  } else {
    logWarn(`Não foi possível listar refunds: ${listRes.status}`);
  }

  result.success = result.steps.create.success && result.steps.list.success;
  printTestResult('REFUND', result.success, result.steps);

  return result;
}

// ============================================================================
// TEARDOWN - Limpeza (Executado uma vez no final)
// ============================================================================
export function teardown(data) {
  const totalDuration = (Date.now() - data.startTime) / 1000;

  console.log('\n');
  printSeparator();
  logInfo(`Teste finalizado em ${totalDuration.toFixed(2)} segundos`);
  logInfo(`Organization: ${data.organizationId}`);
  logInfo(`Ledger: ${data.ledgerId}`);
  logInfo(`Account criada: ${data.accountId}`);
  printSeparator();
}

// ============================================================================
// HANDLE SUMMARY - Relatório Final
// ============================================================================
export function handleSummary(data) {
  const duration = Math.round(data.state.testRunDurationMs / 1000);
  const requests = data.metrics.http_reqs?.values?.count || 0;
  const avgLatency = Math.round(data.metrics.http_req_duration?.values?.avg || 0);
  const failedRate = ((data.metrics.http_req_failed?.values?.rate || 0) * 100).toFixed(2);

  console.log('\n');
  console.log('='.repeat(70));
  console.log('                PIX VALIDATION TEST - RELATÓRIO FINAL');
  console.log('='.repeat(70));
  console.log(`Duration: ${duration}s`);
  console.log(`Total Requests: ${requests}`);
  console.log(`Avg Latency: ${avgLatency}ms`);
  console.log(`Failed Rate: ${failedRate}%`);
  console.log('-'.repeat(70));
  console.log('IDs UTILIZADOS (FIXOS):');
  console.log(`  Organization: ${MIDAZ_ORGANIZATION_ID}`);
  console.log(`  Ledger: ${MIDAZ_LEDGER_ID}`);
  console.log('='.repeat(70));

  return {};
}

// ============================================================================
// FUNÇÕES AUXILIARES DE LOG
// ============================================================================
function printHeader() {
  console.log('\n' + '='.repeat(70));
  console.log('     PIX FULL VALIDATION TEST - IDs Fixos Obrigatórios');
  console.log('='.repeat(70));
}

function printSeparator() {
  console.log('-'.repeat(70));
}

function printTestHeader(title) {
  console.log('\n' + '~'.repeat(70));
  console.log(`  ${title}`);
  console.log('~'.repeat(70));
}

function printTestResult(name, success, steps) {
  console.log('\n' + '-'.repeat(50));
  console.log(`RESULTADO ${name}: ${success ? 'SUCESSO' : 'FALHA'}`);

  for (const [stepName, step] of Object.entries(steps)) {
    const icon = step.success ? '[OK]' : '[FAIL]';
    console.log(`  ${icon} ${stepName}: ${step.status} (${step.duration}ms)`);
  }
  console.log('-'.repeat(50));
}

function printFinalSummary(results) {
  console.log('\n');
  console.log('='.repeat(70));
  console.log('                    SUMÁRIO FINAL DOS TESTES');
  console.log('='.repeat(70));

  const tests = [
    { name: 'Collection', result: results.collection },
    { name: 'Cashout', result: results.cashout },
    { name: 'Refund', result: results.refund }
  ];

  for (const test of tests) {
    if (test.result) {
      const icon = test.result.success ? '[PASS]' : '[FAIL]';
      console.log(`  ${icon} ${test.name}`);
    } else {
      console.log(`  [SKIP] ${test.name}`);
    }
  }

  console.log('='.repeat(70));
}

function logStep(num, message) {
  console.log(`\n[STEP ${num}] ${message}`);
}

function logInfo(message) {
  console.log(`[INFO] ${message}`);
}

function logSuccess(message) {
  console.log(`[SUCCESS] ${message}`);
}

function logError(message) {
  console.error(`[ERROR] ${message}`);
}

function logWarn(message) {
  console.warn(`[WARN] ${message}`);
}

function logDebug(message) {
  if (LOG === 'DEBUG') {
    console.log(`[DEBUG] ${message}`);
  }
}
