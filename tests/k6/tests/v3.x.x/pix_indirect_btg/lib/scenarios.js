/**
 * ============================================================================
 * PIX Test Scenarios - Módulo Compartilhado
 * ============================================================================
 *
 * Cenários de teste reutilizáveis para Collection e Cashout PIX.
 * Este módulo centraliza a lógica comum entre diferentes arquivos de teste.
 *
 * Usage:
 *   import * as scenarios from './lib/scenarios.js';
 *   scenarios.collectionScenario(data, { prefix: 'Complete', log: LOG });
 *   scenarios.cashoutScenario(data, { prefix: 'Complete', log: LOG });
 *
 * ============================================================================
 */

import { sleep } from 'k6';
import * as pix from '../../../../pkg/pix.js';
import * as auth from '../../../../pkg/auth.js';
import * as generators from './generators.js';

// ============================================================================
// TOKEN REFRESH SUPPORT
// ============================================================================
const TOKEN_REFRESH_THRESHOLD_MS = 5 * 60 * 1000;  // Refresh 5 min antes de expirar

// Per-runtime token cache to avoid refresh on every iteration
let cachedToken = null;
let cachedTokenExpiry = 0;

/**
 * Obtém token válido, fazendo refresh se necessário
 * Para testes longos (soak), garante que o token não expire
 */
function getValidToken(data) {
  const now = Date.now();

  // Reuse cached token while still valid
  if (cachedToken && cachedTokenExpiry > now + TOKEN_REFRESH_THRESHOLD_MS) {
    return cachedToken;
  }

  // If setup did not provide explicit expiry, apply a conservative fallback TTL.
  // This avoids fail-open behavior in long-running suites.
  const baseTokenExpiry = data.tokenExpiry || (now + (55 * 60 * 1000));

  // Verifica se o token original ainda é válido
  if (data.token && now < baseTokenExpiry - TOKEN_REFRESH_THRESHOLD_MS) {
    cachedToken = data.token;
    cachedTokenExpiry = baseTokenExpiry;
    return data.token;
  }

  // Token expirado ou próximo de expirar - gerar novo
  console.log(`[Token] VU${__VU}: Refreshing token (expiry approaching)`);
  const newToken = auth.generateToken();
  const newExpiry = now + (55 * 60 * 1000);  // 55 minutos

  cachedToken = newToken;
  cachedTokenExpiry = newExpiry;

  return newToken;
}

/**
 * Cenário de Collection (Cobrança PIX)
 *
 * @param {Object} data - Dados do setup (token, accounts, pixKeys, etc.)
 * @param {Object} options - Opções de configuração
 * @param {string} options.prefix - Prefixo para descrições (default: '')
 * @param {string} options.log - Nível de log: 'DEBUG', 'ERROR', 'OFF' (default: 'OFF')
 * @param {boolean} options.includeHolderId - Incluir holderId no payload (default: false)
 */
export function collectionScenario(data, options = {}) {
  const {
    prefix = '',
    log = 'OFF',
    includeHolderId = false
  } = options;

  const descPrefix = prefix ? `${prefix} ` : '';
  const testIdPrefix = prefix ? `${prefix.toLowerCase()}-` : '';

  if (!data.accounts || data.accounts.length === 0) {
    console.error('[Collection] Nenhuma account disponível');
    return;
  }

  // Seleciona account baseado no VU
  const accountIndex = __VU % data.accounts.length;
  const account = data.accounts[accountIndex];

  // Seleciona PIX key para receiver
  const emailKey = data.pixKeys.emailKeys.find(k => k.accountId === account.id);
  const cpfKey = data.pixKeys.cpfKeys.find(k => k.accountId === account.id);
  const cnpjKeys = Array.isArray(data.pixKeys?.cnpjKeys) ? data.pixKeys.cnpjKeys : [];
  const cnpjKey = cnpjKeys.find(k => k.accountId === account.id);
  const receiverKey = emailKey?.key || cpfKey?.key || cnpjKey?.key || account.document;

  // Gera dados da collection
  const txId = generators.generateTxId(32);
  const idempotencyKey = generators.generateIdempotencyKey();
  const amount = generators.generateAmount(10, 500);
  const debtorDocument = generators.generateCPF();
  const debtorName = generators.generateDebtorName();

  // Payload completo conforme Postman collection
  const payload = JSON.stringify({
    txId: txId,
    receiverKey: receiverKey,
    amount: amount,
    expirationSeconds: 3600,
    debtorDocument: debtorDocument,
    debtorName: debtorName,
    description: `PIX Collection ${descPrefix}- VU ${__VU} ITER ${__ITER}`,
    metadata: {
      source: 'k6_load_test',
      test: 'immediate_collection',
      vu: __VU,
      iter: __ITER
    }
  });

  // Cria collection
  // Obtém token válido (com refresh automático se necessário)
  const token = getValidToken(data);

  const createRes = pix.collection.create(token, account.id, payload, idempotencyKey);

  if (log === 'DEBUG') {
    console.log(`[Collection] Account: ${account.id}, Status: ${createRes.status}`);
  }

  if (createRes.status === 201 || createRes.status === 200) {
    let body;
    try {
      body = JSON.parse(createRes.body);
    } catch (e) {
      console.error(`[Collection] Failed to parse response: ${e.message}`);
      return;
    }

    // Consulta a collection criada
    sleep(0.5);
    const getRes = pix.collection.getById(token, account.id, body.id);

    if (log === 'DEBUG') {
      console.log(`[Collection Get] Status: ${getRes.status}`);
    }

    // Deleta a collection (cleanup)
    sleep(0.5);
    pix.collection.remove(token, account.id, body.id, 'DELETED_BY_USER');
  }

  sleep(generators.getThinkTime('betweenOperations'));
}

/**
 * Cenário de Cashout (Pagamento PIX)
 *
 * @param {Object} data - Dados do setup (token, accounts, pixKeys, etc.)
 * @param {Object} options - Opções de configuração
 * @param {string} options.prefix - Prefixo para descrições (default: '')
 * @param {string} options.log - Nível de log: 'DEBUG', 'ERROR', 'OFF' (default: 'OFF')
 * @param {boolean} options.useTargetAccount - Usar conta diferente como destino (default: false)
 * @param {string} options.randomKeyType - Nome do tipo de chave aleatória: 'RANDOM' ou 'EVP' (default: 'RANDOM')
 */
export function cashoutScenario(data, options = {}) {
  const {
    prefix = '',
    log = 'OFF',
    useTargetAccount = false,
    randomKeyType = 'RANDOM'
  } = options;

  const descPrefix = prefix ? `${prefix} ` : '';

  if (!data.accounts || data.accounts.length === 0) {
    console.error('[Cashout] Nenhuma account disponível');
    return;
  }

  // Seleciona account baseado no VU
  const accountIndex = __VU % data.accounts.length;
  const account = data.accounts[accountIndex];

  // Seleciona tipo de PIX key para initiate
  const keyTypes = ['EMAIL', 'PHONE', 'CPF', randomKeyType];
  if (data.pixKeys.cnpjKeys && data.pixKeys.cnpjKeys.length > 0) {
    keyTypes.push('CNPJ');
  }
  const keyType = keyTypes[__ITER % keyTypes.length];

  // Seleciona PIX key baseado no tipo
  let pixKey;
  const keyIndex = useTargetAccount
    ? (accountIndex + 1) % data.accounts.length
    : __ITER;

  switch (keyType) {
    case 'EMAIL':
      pixKey = useTargetAccount
        ? data.pixKeys.emailKeys[keyIndex]?.key
        : data.pixKeys.emailKeys[keyIndex % data.pixKeys.emailKeys.length]?.key;
      break;
    case 'PHONE':
      pixKey = data.pixKeys.phoneKeys[keyIndex % Math.max(data.pixKeys.phoneKeys.length, 1)]?.key;
      break;
    case 'CPF':
      pixKey = data.pixKeys.cpfKeys[keyIndex % Math.max(data.pixKeys.cpfKeys.length, 1)]?.key;
      break;
    case 'CNPJ':
      pixKey = data.pixKeys.cnpjKeys[keyIndex % Math.max(data.pixKeys.cnpjKeys.length, 1)]?.key;
      break;
    case 'RANDOM':
    case 'EVP':
      pixKey = data.pixKeys.randomKeys[keyIndex % Math.max(data.pixKeys.randomKeys.length, 1)]?.key;
      break;
  }

  // Fallback para email se não encontrar
  if (!pixKey) {
    pixKey = data.pixKeys.emailKeys[0]?.key || generators.generateEmailKey();
  }

  // Obtém token válido (com refresh automático se necessário)
  const token = getValidToken(data);

  // Step 1: Initiate
  const initiateIdempotency = generators.generateIdempotencyKey();
  const initiatePayload = JSON.stringify({
    initiationType: 'KEY',
    key: pixKey,
    description: `PIX Cashout ${descPrefix}- VU ${__VU} ITER ${__ITER} - ${keyType}`
  });

  const initiateRes = pix.transfer.initiate(token, account.id, initiatePayload, initiateIdempotency);

  if (log === 'DEBUG') {
    console.log(`[Cashout Initiate] Account: ${account.id}, KeyType: ${keyType}, Status: ${initiateRes.status}`);
  }

  if (initiateRes.status === 201 || initiateRes.status === 200) {
    let initiateBody;
    try {
      initiateBody = JSON.parse(initiateRes.body);
    } catch (e) {
      console.error(`[Cashout] Failed to parse initiate response: ${e.message}`);
      return;
    }

    // Simula tempo de confirmação do usuário
    sleep(generators.getThinkTime('userConfirmation'));

    // Step 2: Process
    const processIdempotency = generators.generateIdempotencyKey();
    const amount = generators.generateAmount(10, 100);

    const processPayload = JSON.stringify({
      initiationId: initiateBody.id,
      amount: amount,
      description: `PIX Cashout Process ${descPrefix}- VU ${__VU}`,
      metadata: {
        source: 'k6_load_test',
        test: 'cashout_transfer',
        vu: __VU,
        iter: __ITER
      }
    });

    const processRes = pix.transfer.process(token, account.id, processPayload, processIdempotency);

    if (log === 'DEBUG') {
      console.log(`[Cashout Process] Status: ${processRes.status}`);
    }
  }

  sleep(generators.getThinkTime('betweenOperations'));
}
