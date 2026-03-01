/**
 * ============================================================================
 * PIX COMPLETE SETUP - Fluxo Completo de Criação de Entidades
 * ============================================================================
 *
 * Este script cria todas as entidades necessárias para testes de PIX completos,
 * following the correct flow linked to default Organization and Ledger IDs.
 *
 * DEFAULT IDs (can be overridden by environment):
 *   MIDAZ_ORGANIZATION_ID = 019be10f-df74-78ce-ac1c-0ef1e8d810fb
 *   MIDAZ_LEDGER_ID       = 019be10f-fa03-77a3-b395-aa8c7974a2c0
 *
 * RESTRIÇÕES:
 *   ❌ DON'T create Organization during setup (use configured ID)
 *   ❌ DON'T create Ledger during setup (use configured ID)
 *   ❌ NÃO criar Asset BRL (usar existente)
 *   ✅ CREATE Account (bound to configured Org/Ledger)
 *   ✅ CREATE Holder (bound to configured Organization)
 *   ✅ CRIAR Alias (vinculado ao Holder/Account)
 *   ✅ CRIAR DICT Entry (chave PIX vinculada à Account)
 *
 * FLUXO CORRETO:
 *   1. Criar Account (Midaz Onboarding)
 *   2. Criar Holder (CRM)
 *   3. Criar Alias (CRM) - vinculando Holder com Account
 *   4. Criar DICT Entry (PIX) - registrar chave PIX
 *   5. Retornar dados para uso nos testes
 *
 * VARIÁVEIS DE AMBIENTE:
 *   - ENVIRONMENT: dev, sandbox, vpc (default: dev)
 *   - NUM_ACCOUNTS: Número de contas a criar (default: 5)
 *   - LOG: DEBUG, ERROR, OFF (default: OFF)
 *
 * USAGE (standalone):
 *   k6 run setup/pix-complete-setup.js -e NUM_ACCOUNTS=10
 *
 * ============================================================================
 */

import * as auth from '../../../../pkg/auth.js';
import * as midaz from '../../../../pkg/midaz.js';
import * as crm from '../../../../pkg/crm.js';
import * as pix from '../../../../pkg/pix.js';
import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';
import {
  generateCPF as generateCPFValue,
  generateCNPJ as generateCNPJValue
} from '../../../../helper/dataGenerators.js';

// ============================================================================
// IDs FIXOS OBRIGATÓRIOS - NÃO ALTERAR
// ============================================================================
const DEFAULT_MIDAZ_ORGANIZATION_ID = '019be10f-df74-78ce-ac1c-0ef1e8d810fb';
const DEFAULT_MIDAZ_LEDGER_ID = '019be10f-fa03-77a3-b395-aa8c7974a2c0';
export const MIDAZ_ORGANIZATION_ID = __ENV.MIDAZ_ORGANIZATION_ID || DEFAULT_MIDAZ_ORGANIZATION_ID;
export const MIDAZ_LEDGER_ID = __ENV.MIDAZ_LEDGER_ID || DEFAULT_MIDAZ_LEDGER_ID;

// ============================================================================
// CONFIGURAÇÕES
// ============================================================================
const ENVIRONMENT = __ENV.ENVIRONMENT || 'dev';
const NUM_ACCOUNTS = parseInt(__ENV.NUM_ACCOUNTS || '5', 10);
const LOG = (__ENV.LOG || 'OFF').toUpperCase();
const INCLUDE_CNPJ_FLOW = __ENV.K6_INCLUDE_CNPJ_FLOW === 'true';
const SETUP_MIN_SUCCESS_RATE = Number(__ENV.K6_SETUP_MIN_SUCCESS_RATE || '1');
const ALLOW_FIXED_IDS_OUTSIDE_DEV = __ENV.K6_ALLOW_FIXED_TENANT_IDS === 'true';

if (ENVIRONMENT !== 'dev' && !ALLOW_FIXED_IDS_OUTSIDE_DEV) {
  if (!__ENV.MIDAZ_ORGANIZATION_ID || !__ENV.MIDAZ_LEDGER_ID) {
    throw new Error(
      'For non-dev environments, MIDAZ_ORGANIZATION_ID and MIDAZ_LEDGER_ID must be explicitly provided. ' +
      'Use K6_ALLOW_FIXED_TENANT_IDS=true only for dedicated fixture environments.'
    );
  }
}

// ============================================================================
// K6 OPTIONS (para execução standalone)
// ============================================================================
export const options = {
  scenarios: {
    setup_only: {
      exec: 'runSetup',
      executor: 'shared-iterations',
      vus: 1,
      iterations: 1,
      maxDuration: '10m'
    }
  },
  thresholds: {
    http_req_duration: ['p(95)<3000'],  // Mais rigoroso: 3s ao invés de 5s
    http_req_failed: ['rate<0.15']       // Mais rigoroso: 15% ao invés de 30%
  }
};

// ============================================================================
// CONFIGURAÇÕES DE TOKEN
// ============================================================================
const TOKEN_VALIDITY_MS = 55 * 60 * 1000;  // 55 minutos (margem de segurança)
const TOKEN_REFRESH_THRESHOLD_MS = 5 * 60 * 1000;  // Refresh 5 min antes de expirar

// ============================================================================
// GERADORES DE DADOS
// ============================================================================

function generateCPF() {
  return generateCPFValue();
}

function generateCNPJ() {
  return generateCNPJValue();
}

function generateEmail(index) {
  const timestamp = Date.now();
  const domains = ['gmail.com', 'hotmail.com', 'yahoo.com.br'];
  const domain = domains[index % domains.length];
  // Simplified email format without special characters
  return `pixtest${String(index).padStart(3, '0')}${timestamp}@${domain}`;
}

function generatePhone(index) {
  const ddds = ['11', '21', '31', '41', '51', '61', '71', '81', '85', '92'];
  const ddd = ddds[index % ddds.length];
  const timestamp = Date.now().toString().slice(-6);
  const paddedIndex = String(index).padStart(2, '0');
  return `+55${ddd}9${paddedIndex}${timestamp}`;
}

function generateRandomKey() {
  return uuidv4();
}

function generateName(isCompany) {
  if (isCompany) {
    const companyNames = ['Comercial', 'Distribuidora', 'Servicos', 'Solucoes', 'Tecnologia', 'Logistica', 'Consultoria', 'Industria'];
    const adjectives = ['Brasil', 'Nacional', 'Global', 'Premium', 'Prime', 'Master', 'Express', 'Central'];
    const suffixes = ['Ltda', 'S.A.', 'ME', 'Eireli'];
    const companyName = companyNames[Math.floor(Math.random() * companyNames.length)];
    const adjective = adjectives[Math.floor(Math.random() * adjectives.length)];
    const suffix = suffixes[Math.floor(Math.random() * suffixes.length)];
    return `${companyName} ${adjective} ${suffix}`;
  }
  const firstNames = ['Joao', 'Maria', 'Pedro', 'Ana', 'Carlos', 'Julia', 'Lucas', 'Laura'];
  const lastNames = ['Silva', 'Santos', 'Oliveira', 'Souza', 'Rodrigues', 'Ferreira', 'Alves', 'Pereira'];
  return `${firstNames[Math.floor(Math.random() * firstNames.length)]} ${lastNames[Math.floor(Math.random() * lastNames.length)]}`;
}

function parseJsonResponse(res, operationName) {
  try {
    return JSON.parse(res.body);
  } catch (error) {
    console.error(`[SETUP] ${operationName}: invalid JSON response (${error.message})`);
    return null;
  }
}

function maskSensitive(value, prefix = 2, suffix = 2) {
  if (typeof value !== 'string' || value.length <= prefix + suffix) {
    return '***';
  }

  return `${value.slice(0, prefix)}***${value.slice(-suffix)}`;
}

// ============================================================================
// FUNÇÕES DE CRIAÇÃO
// ============================================================================

/**
 * Step 1: Criar Account no Midaz Onboarding
 */
function createAccount(token, index, document) {
  // Gerar name e alias dinâmicos sem números no nome, mas com sufixo único no alias
  const adjectives = ['Alpha', 'Beta', 'Gamma', 'Delta', 'Epsilon', 'Zeta', 'Eta', 'Theta', 'Iota', 'Kappa'];
  const nouns = ['Account', 'Wallet', 'Deposit', 'Savings', 'Current', 'Primary', 'Secondary', 'Main', 'Reserve', 'Fund'];

  const adjective = adjectives[index % adjectives.length];
  const noun = nouns[Math.floor(index / adjectives.length) % nouns.length];
  const uniqueSuffix = Date.now().toString().slice(-6);

  const accountName = `${adjective} ${noun}`;
  const accountAlias = `@${adjective.toLowerCase()}_${noun.toLowerCase()}_${uniqueSuffix}`;

  const payload = JSON.stringify({
    assetCode: 'BRL',
    name: accountName,
    alias: accountAlias,
    type: 'deposit',
    status: {
      code: 'ACTIVE',
      description: 'Account created by k6 setup'
    },
    metadata: {
      source: 'k6_pix_setup',
      role: 'sender'
    }
  });

  const res = midaz.account.create(token, MIDAZ_ORGANIZATION_ID, MIDAZ_LEDGER_ID, payload);

  if (res.status === 201 || res.status === 200) {
    const body = parseJsonResponse(res, 'createAccount');
    if (!body || !body.id) {
      return { success: false, status: res.status, error: 'createAccount returned invalid payload' };
    }

    return {
      success: true,
      account: {
        id: body.id,
        name: accountName,
        alias: accountAlias,
        document: document
      }
    };
  } else {
    return { success: false, status: res.status, error: res.body };
  }
}

/**
 * Step 2: Criar Holder no CRM
 * Apenas document, mobilePhone e primaryEmail são dinâmicos
 */
function createHolder(token, document, index, email, phone, holderType = 'NATURAL_PERSON') {
  const externalId = uuidv4();
  const isNaturalPerson = holderType === 'NATURAL_PERSON';
  const holderName = isNaturalPerson ? 'Jose Maria Test' : `Empresa Teste ${index}`;

  const payloadBase = {
    name: holderName,
    document: document,
    type: holderType,
    externalId: externalId,
    contact: {
      mobilePhone: phone,
      primaryEmail: email
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
      source: 'k6_pix_setup',
      role: 'sender'
    }
  };

  if (isNaturalPerson) {
    payloadBase.naturalPerson = {
      birthDate: '1990-01-15',
      civilStatus: 'Single',
      gender: 'Male',
      nationality: 'Brazilian',
      favoriteName: 'Holder A',
      socialName: 'Jose Maria Test',
      motherName: 'Maria Test',
      fatherName: 'Jose Test',
      status: 'Active'
    };
  } else {
    payloadBase.legalPerson = {
      corporateName: holderName,
      tradeName: `Empresa ${index}`,
      incorporationDate: '2015-01-01',
      legalNature: 'LIMITED_COMPANY',
      status: 'Active'
    };
  }

  const payload = JSON.stringify(payloadBase);

  const res = crm.holder.create(token, MIDAZ_ORGANIZATION_ID, payload);

  if (res.status === 201 || res.status === 200) {
    const body = parseJsonResponse(res, 'createHolder');
    if (!body || !body.id) {
      return { success: false, status: res.status, error: 'createHolder returned invalid payload' };
    }

    return {
      success: true,
        holder: {
          id: body.id,
          name: holderName,
          document: document,
          type: holderType,
          email: email,
          phone: phone,
          externalId: externalId
      }
    };
  } else {
    return { success: false, status: res.status, error: res.body };
  }
}

/**
 * Step 3: Criar Alias no CRM (vincula Holder com Account)
 * Apenas accountId, ledgerId e account (número) são dinâmicos
 */
function createAlias(token, holderId, accountId, index) {
  // Apenas account number é dinâmico, demais campos fixos
  const accountNumber = String(100000 + index);

  const payload = JSON.stringify({
    accountId: accountId,
    ledgerId: MIDAZ_LEDGER_ID,
    bankingDetails: {
      account: accountNumber,
      bankId: '13866572',
      branch: '0001',
      countryCode: 'BR',
      iban: 'ME59406160025300106274',
      openingDate: '2025-09-05',
      type: 'TRAN'
    },
    metadata: {
      source: 'k6_pix_setup',
      role: 'sender'
    }
  });

  const res = crm.alias.create(token, MIDAZ_ORGANIZATION_ID, holderId, payload);

  if (res.status === 201 || res.status === 200) {
    const body = parseJsonResponse(res, 'createAlias');
    if (!body || !body.id) {
      return { success: false, status: res.status, error: 'createAlias returned invalid payload' };
    }

    return {
      success: true,
      alias: {
        id: body.id,
        holderId: holderId,
        accountId: accountId,
        accountNumber: accountNumber
      }
    };
  } else {
    return { success: false, status: res.status, error: res.body };
  }
}

/**
 * Step 4: Criar DICT Entry (registrar chave PIX)
 * For EVP keys, the key is auto-generated by the API (don't send key field)
 */
function createDictEntry(token, accountId, key, keyType, reason = 'USER_REQUESTED') {
  // Log payload for debugging
  if (LOG === 'DEBUG') {
    const maskedKey = key ? maskSensitive(String(key), 2, 2) : 'AUTO';
    console.log(`[DICT] Creating entry: key=${maskedKey}, keyType=${keyType}`);
  }

  // EVP keys are auto-generated - don't send the key field
  let payload;
  if (keyType === 'EVP') {
    payload = JSON.stringify({ keyType: keyType });
  } else {
    payload = JSON.stringify({ key: key, keyType: keyType });
  }

  const res = pix.dict.create(token, accountId, payload, reason);

  if (res.status === 201 || res.status === 200) {
    const body = parseJsonResponse(res, 'createDictEntry');
    if (!body || !body.id) {
      return { success: false, status: res.status, error: 'createDictEntry returned invalid payload' };
    }

    // For EVP keys, the key is returned in the response
    const actualKey = body.key || key;
    return {
      success: true,
      dictEntry: {
        id: body.id,
        key: actualKey,
        keyType: keyType,
        accountId: accountId,
        status: body.status
      }
    };
  } else {
    return { success: false, status: res.status, error: res.body };
  }
}

// ============================================================================
// FUNÇÕES DE CARGA DE SALDO
// ============================================================================

/**
 * Step 0: Criar External Account (@external/BRL)
 */
function createExternalAccount(token) {
  // First, try to get the external account to see if it already exists
  // Use URL-encoded alias: @external/BRL -> %40external%2FBRL
  const encodedAlias = encodeURIComponent('@external/BRL');
  const getRes = midaz.account.getByAlias(token, MIDAZ_ORGANIZATION_ID, MIDAZ_LEDGER_ID, encodedAlias);

  if (getRes.status === 200) {
    return { success: true, alreadyExists: true };
  }

  // If not found, create it with type 'deposit' (external is a special alias, not a type)
  const payload = JSON.stringify({
    assetCode: 'BRL',
    name: 'External BRL Account',
    alias: '@external/BRL',
    type: 'deposit',
    status: {
      code: 'ACTIVE',
      description: 'External account for fund transfers'
    }
  });

  const res = midaz.account.create(token, MIDAZ_ORGANIZATION_ID, MIDAZ_LEDGER_ID, payload);

  if (res.status === 201 || res.status === 200) {
    return { success: true };
  } else if (res.status === 409) {
    return { success: true, alreadyExists: true };
  } else {
    // If creation fails with 400, assume the external account already exists
    // as a system account or with different configuration
    if (res.status === 400) {
      console.warn(`[SETUP] External account creation returned 400, assuming system account exists`);
      return { success: true, alreadyExists: true };
    }
    return { success: false, status: res.status, error: res.body };
  }
}

/**
 * Step 1.5: Criar Balance na conta
 */
function createBalance(token, accountId, balanceKey = 'default') {
  const payload = JSON.stringify({
    key: balanceKey
  });

  const res = midaz.balance.create(token, MIDAZ_ORGANIZATION_ID, MIDAZ_LEDGER_ID, accountId, payload);

  if (res.status === 201 || res.status === 200) {
    return { success: true };
  } else if (res.status === 409) {
    return { success: true, alreadyExists: true };
  } else {
    return { success: false, status: res.status, error: res.body };
  }
}

/**
 * Step 1.6: Carregar saldo na conta (R$ 10.000,00)
 */
function chargeBalance(token, accountAlias, amount = '1000000') {
  const idempotencyKey = uuidv4();
  const requestId = uuidv4();

  const payload = JSON.stringify({
    send: {
      asset: 'BRL',
      value: amount,
      source: {
        from: [
          {
            accountAlias: '@external/BRL',
            amount: {
              asset: 'BRL',
              value: amount
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
              value: amount
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
      source: 'k6_pix_complete_setup'
    }
  });

  const res = midaz.transaction.create(token, MIDAZ_ORGANIZATION_ID, MIDAZ_LEDGER_ID, payload, idempotencyKey, requestId);

  if (res.status === 201 || res.status === 200) {
    return { success: true };
  } else {
    return { success: false, status: res.status, error: res.body };
  }
}

// ============================================================================
// SETUP COMPLETO
// ============================================================================

export function executeCompleteSetup(token, numAccounts) {
  const result = {
    success: false,
    organizationId: MIDAZ_ORGANIZATION_ID,
    ledgerId: MIDAZ_LEDGER_ID,
    accounts: [],
    holders: [],
    aliases: [],
    dictEntries: [],
    pixKeys: {
      emailKeys: [],
      phoneKeys: [],
      cpfKeys: [],
      cnpjKeys: [],
      randomKeys: []
    },
    errors: []
  };

  // Step 0: Criar External Account (@external/BRL)
  console.log('\n[SETUP] Step 0: Criando External Account (@external/BRL)...');
  const externalResult = createExternalAccount(token);
  if (externalResult.success) {
    if (externalResult.alreadyExists) {
      console.log('[SETUP] External Account @external/BRL já existe - OK');
    } else {
      console.log('[SETUP] External Account @external/BRL criada');
    }
  } else {
    console.error(`[SETUP] Falha ao criar External Account: ${externalResult.error}`);
    result.errors.push({ step: 'external_account', error: externalResult.error });
  }

  // Steps 1-4: Criar entidades para cada conta
  console.log(`\n[SETUP] Steps 1-4: Criando ${numAccounts} entidades completas...`);

  for (let i = 1; i <= numAccounts; i++) {
    console.log(`\n[SETUP] === Entidade ${i}/${numAccounts} ===`);

    const useCompanyFlow = INCLUDE_CNPJ_FLOW && i % 2 === 0;
    const holderType = useCompanyFlow ? 'LEGAL_PERSON' : 'NATURAL_PERSON';
    const document = useCompanyFlow ? generateCNPJ() : generateCPF();

    // Gerar email/phone uma vez para reutilizar em Holder e DICT
    const entityEmail = generateEmail(i);
    const entityPhone = generatePhone(i);

    // Step 1: Criar Account
    console.log(`[SETUP] Step 1.${i}: Criando Account...`);
    const accountResult = createAccount(token, i, document);

    if (!accountResult.success) {
      console.error(`[SETUP] Falha ao criar Account ${i}: ${accountResult.error}`);
      result.errors.push({ step: `account_${i}`, error: accountResult.error });
      continue;
    }

    const account = accountResult.account;
    result.accounts.push(account);
    console.log(`[SETUP] Account criada: ${account.id}`);

    // Step 1.5: Criar Balance na conta
    console.log(`[SETUP] Step 1.5.${i}: Criando Balance...`);
    const balanceResult = createBalance(token, account.id, 'default');
    if (balanceResult.success) {
      console.log(`[SETUP] Balance "default" criado para Account ${account.id}`);
    } else {
      console.warn(`[SETUP] Aviso: Falha ao criar Balance: ${balanceResult.error}`);
      result.errors.push({ step: `balance_${i}`, error: balanceResult.error });
    }

    // Step 1.6: Carregar saldo na conta (R$ 10.000,00)
    console.log(`[SETUP] Step 1.6.${i}: Carregando saldo (R$ 10.000,00)...`);
    const chargeResult = chargeBalance(token, account.alias, '1000000');
    if (chargeResult.success) {
      console.log(`[SETUP] Saldo carregado: R$ 10.000,00 para Account ${account.alias}`);
    } else {
      console.warn(`[SETUP] Aviso: Falha ao carregar saldo: ${chargeResult.error}`);
      result.errors.push({ step: `charge_${i}`, error: chargeResult.error });
    }

    // Step 2: Criar Holder
    console.log(`[SETUP] Step 2.${i}: Criando Holder...`);
    const holderResult = createHolder(token, document, i, entityEmail, entityPhone, holderType);

    if (!holderResult.success) {
      console.error(`[SETUP] Falha ao criar Holder ${i}: ${holderResult.error}`);
      result.errors.push({ step: `holder_${i}`, error: holderResult.error });
      result.accounts.pop(); // Remove the orphaned account
      continue;
    }

    const holder = holderResult.holder;
    result.holders.push(holder);
    console.log(`[SETUP] Holder criado: ${holder.id}`);

    // Step 3: Criar Alias
    console.log(`[SETUP] Step 3.${i}: Criando Alias...`);
    const aliasResult = createAlias(token, holder.id, account.id, i);

    if (!aliasResult.success) {
      console.error(`[SETUP] Falha ao criar Alias ${i}: ${aliasResult.error}`);
      result.errors.push({ step: `alias_${i}`, error: aliasResult.error });
    } else {
      result.aliases.push(aliasResult.alias);
      console.log(`[SETUP] Alias criado: ${aliasResult.alias.id}`);
    }

    // Step 4: Criar DICT Entries (EMAIL, CPF/CNPJ e PHONE)
    const taxKeyType = holderType === 'LEGAL_PERSON' ? 'CNPJ' : 'CPF';
    console.log(`[SETUP] Step 4.${i}: Criando DICT Entries (EMAIL, ${taxKeyType}, PHONE)...`);

    // EMAIL key
    const emailResult = createDictEntry(token, account.id, entityEmail, 'EMAIL');
    if (emailResult.success) {
      result.dictEntries.push(emailResult.dictEntry);
      result.pixKeys.emailKeys.push({
        key: entityEmail,
        type: 'EMAIL',
        accountId: account.id,
        dictEntryId: emailResult.dictEntry.id
      });
      console.log(`[SETUP] DICT Entry EMAIL criada: ${maskSensitive(entityEmail, 2, 6)}`);
    } else {
      result.errors.push({ step: `dict_email_${i}`, error: emailResult.error });
    }

    // Tax document key (CPF for natural person, CNPJ for legal person)
    const taxDocResult = createDictEntry(token, account.id, document, taxKeyType);
    if (taxDocResult.success) {
      result.dictEntries.push(taxDocResult.dictEntry);

      const keyBucket = taxKeyType === 'CPF' ? result.pixKeys.cpfKeys : result.pixKeys.cnpjKeys;
      keyBucket.push({
        key: document,
        type: taxKeyType,
        accountId: account.id,
        dictEntryId: taxDocResult.dictEntry.id
      });

      console.log(`[SETUP] DICT Entry ${taxKeyType} criada: ${maskSensitive(document, 3, 2)}`);
    } else {
      result.errors.push({ step: `dict_${taxKeyType.toLowerCase()}_${i}`, error: taxDocResult.error });
    }

    // PHONE key
    const phoneResult = createDictEntry(token, account.id, entityPhone, 'PHONE');
    if (phoneResult.success) {
      result.dictEntries.push(phoneResult.dictEntry);
      result.pixKeys.phoneKeys.push({
        key: entityPhone,
        type: 'PHONE',
        accountId: account.id,
        dictEntryId: phoneResult.dictEntry.id
      });
      console.log(`[SETUP] DICT Entry PHONE criada: ${maskSensitive(entityPhone, 4, 2)}`);
    } else {
      result.errors.push({ step: `dict_phone_${i}`, error: phoneResult.error });
    }

    // EVP (Random) key - Auto-generated by the API
    const evpResult = createDictEntry(token, account.id, null, 'EVP');
    if (evpResult.success) {
      result.dictEntries.push(evpResult.dictEntry);
      result.pixKeys.randomKeys.push({
        key: evpResult.dictEntry.key,
        type: 'EVP',
        accountId: account.id,
        dictEntryId: evpResult.dictEntry.id
      });
      console.log(`[SETUP] DICT Entry EVP criada: ${maskSensitive(String(evpResult.dictEntry.key), 4, 4)}`);
    } else {
      result.errors.push({ step: `dict_evp_${i}`, error: evpResult.error });
    }

    if (LOG === 'DEBUG') {
      console.log(`[SETUP] Entidade ${i} completa!`);
    }
  }

  // Resumo
  console.log('\n[SETUP] ========== RESUMO ==========');
  console.log(`  Accounts criadas: ${result.accounts.length}/${numAccounts}`);
  console.log(`  Holders criados: ${result.holders.length}`);
  console.log(`  Aliases criados: ${result.aliases.length}`);
  console.log(`  DICT Entries: ${result.dictEntries.length}`);
  console.log('  PIX Keys:');
  console.log(`    - Email: ${result.pixKeys.emailKeys.length}`);
  console.log(`    - Phone: ${result.pixKeys.phoneKeys.length}`);
  console.log(`    - CPF: ${result.pixKeys.cpfKeys.length}`);
  console.log(`    - CNPJ: ${result.pixKeys.cnpjKeys.length}`);
  console.log(`    - EVP/Random: ${result.pixKeys.randomKeys.length}`);
  console.log(`  Erros: ${result.errors.length}`);
  console.log('====================================\n');

  // Setup must be deterministic by default (100% success).
  // Override only when intentionally running degraded environment experiments.
  const minSuccessRate = Number.isFinite(SETUP_MIN_SUCCESS_RATE) ? SETUP_MIN_SUCCESS_RATE : 1;
  const accountSuccessRate = result.accounts.length / numAccounts;
  const holderSuccessRate = result.holders.length / numAccounts;

  result.success = accountSuccessRate >= minSuccessRate && holderSuccessRate >= minSuccessRate;

  if (!result.success) {
    console.error(`[SETUP] FALHA: Taxa de sucesso abaixo do mínimo (${(minSuccessRate * 100).toFixed(0)}%)`);
    console.error(`[SETUP]   Accounts: ${result.accounts.length}/${numAccounts} (${(accountSuccessRate * 100).toFixed(1)}%)`);
    console.error(`[SETUP]   Holders: ${result.holders.length}/${numAccounts} (${(holderSuccessRate * 100).toFixed(1)}%)`);
  }

  return result;
}

// ============================================================================
// FUNÇÕES EXPORTADAS
// ============================================================================

/**
 * Gera um novo token de autenticação
 * Pode ser usado para refresh durante testes longos (soak)
 */
export function refreshToken() {
  return auth.generateToken();
}

/**
 * Verifica se o token precisa ser renovado
 * @param {number} tokenExpiry - Timestamp de expiração do token
 * @returns {boolean} - true se precisa renovar
 */
export function shouldRefreshToken(tokenExpiry) {
  return Date.now() > (tokenExpiry - TOKEN_REFRESH_THRESHOLD_MS);
}

/**
 * Cleanup resources created by setup.
 *
 * Best-effort strategy:
 * 1. Remove DICT entries
 * 2. Remove aliases
 * 3. Remove holders
 *
 * Accounts are intentionally left as-is because no delete API is currently wrapped.
 */
export function cleanupSetupResources(data) {
  if (!data || !data.token) {
    return {
      success: false,
      reason: 'missing token or setup data'
    };
  }

  const token = data.token;
  const dictEntries = Array.isArray(data.dictEntries) ? data.dictEntries : [];
  const aliases = Array.isArray(data.aliases) ? data.aliases : [];
  const holders = Array.isArray(data.holders) ? data.holders : [];
  const accounts = Array.isArray(data.accounts) ? data.accounts : [];

  let dictRemoved = 0;
  let aliasRemoved = 0;
  let holderRemoved = 0;
  let accountRemoved = 0;

  for (let i = dictEntries.length - 1; i >= 0; i--) {
    const entry = dictEntries[i];
    if (!entry?.id || !entry?.accountId) {
      continue;
    }

    const res = pix.dict.remove(token, entry.accountId, entry.id, 'CLOSE_ACCOUNT');
    if ([200, 202, 204, 404].includes(res.status)) {
      dictRemoved++;
    }
  }

  for (let i = aliases.length - 1; i >= 0; i--) {
    const alias = aliases[i];
    if (!alias?.id || !alias?.holderId) {
      continue;
    }

    const res = crm.alias.remove(token, MIDAZ_ORGANIZATION_ID, alias.holderId, alias.id);
    if ([200, 202, 204, 404].includes(res.status)) {
      aliasRemoved++;
    }
  }

  for (let i = holders.length - 1; i >= 0; i--) {
    const holder = holders[i];
    if (!holder?.id) {
      continue;
    }

    const res = crm.holder.remove(token, MIDAZ_ORGANIZATION_ID, holder.id);
    if ([200, 202, 204, 404].includes(res.status)) {
      holderRemoved++;
    }
  }

  for (let i = accounts.length - 1; i >= 0; i--) {
    const account = accounts[i];
    if (!account?.id) {
      continue;
    }

    const res = midaz.account.remove(token, MIDAZ_ORGANIZATION_ID, MIDAZ_LEDGER_ID, account.id);
    if ([200, 202, 204, 404].includes(res.status)) {
      accountRemoved++;
    }
  }

  const summary = {
    success:
      dictRemoved === dictEntries.length &&
      aliasRemoved === aliases.length &&
      holderRemoved === holders.length &&
      accountRemoved === accounts.length,
    dictRemoved,
    dictExpected: dictEntries.length,
    aliasRemoved,
    aliasExpected: aliases.length,
    holderRemoved,
    holderExpected: holders.length,
    accountRemoved,
    accountExpected: accounts.length
  };

  console.log(`[TEARDOWN] Cleanup finished: dict=${dictRemoved}/${dictEntries.length}, aliases=${aliasRemoved}/${aliases.length}, holders=${holderRemoved}/${holders.length}, accounts=${accountRemoved}/${accounts.length}`);
  return summary;
}

/**
 * Setup padrão para testes PIX completos
 */
export function defaultSetup(numAccounts = 5) {
  const token = auth.generateToken();
  const tokenExpiry = Date.now() + TOKEN_VALIDITY_MS;

  console.log('\n' + '='.repeat(70));
  console.log('     PIX COMPLETE SETUP - Fluxo Completo de Entidades');
  console.log('='.repeat(70));
  console.log(`Organization ID: ${MIDAZ_ORGANIZATION_ID} (FIXO - NÃO CRIAR)`);
  console.log(`Ledger ID: ${MIDAZ_LEDGER_ID} (FIXO - NÃO CRIAR)`);
  console.log(`Entidades a criar: ${numAccounts}`);
  console.log('Fluxo: Account -> Holder -> Alias -> DICT');
  console.log(`Token válido por: ${TOKEN_VALIDITY_MS / 60000} minutos`);
  console.log('='.repeat(70));

  const setupData = executeCompleteSetup(token, numAccounts);

  console.log('='.repeat(70));
  if (setupData.success) {
    console.log('     SETUP COMPLETO - Pronto para testes PIX');
  } else {
    console.log('     SETUP COM ERROS - Verifique os logs');
  }
  console.log('='.repeat(70) + '\n');

  return {
    token,
    tokenExpiry,
    ...setupData,
    startTime: Date.now()
  };
}

// ============================================================================
// EXECUÇÃO STANDALONE
// ============================================================================

export function setup() {
  return defaultSetup(NUM_ACCOUNTS);
}

export function runSetup(data) {
  console.log('\n[RESULTADO] Setup completo executado!');
  console.log(`  Organization: ${data.organizationId} (FIXO)`);
  console.log(`  Ledger: ${data.ledgerId} (FIXO)`);
  console.log(`  Accounts: ${data.accounts.length}`);
  console.log(`  Holders: ${data.holders.length}`);
  console.log(`  Aliases: ${data.aliases.length}`);
  console.log(`  DICT Entries: ${data.dictEntries.length}`);
  console.log(`  Total PIX Keys: ${Object.values(data.pixKeys).flat().length}`);
}

export function teardown(data) {
  const duration = data && data.startTime
    ? ((Date.now() - data.startTime) / 1000).toFixed(2)
    : 'N/A';

  console.log('\n' + '='.repeat(70));
  console.log('              PIX COMPLETE SETUP - RELATÓRIO FINAL');
  console.log('='.repeat(70));
  console.log(`Duração: ${duration}s`);
  console.log(`Organization: ${data.organizationId} (FIXO - não criado)`);
  console.log(`Ledger: ${data.ledgerId} (FIXO - não criado)`);
  console.log('-'.repeat(70));
  console.log('ENTIDADES CRIADAS:');
  console.log(`  Accounts: ${data.accounts.length}`);
  console.log(`  Holders: ${data.holders.length}`);
  console.log(`  Aliases: ${data.aliases.length}`);
  console.log(`  DICT Entries: ${data.dictEntries.length}`);
  console.log('-'.repeat(70));
  console.log('PIX KEYS REGISTRADAS:');
  console.log(`  Email: ${data.pixKeys.emailKeys.length}`);
  console.log(`  Phone: ${data.pixKeys.phoneKeys.length}`);
  console.log(`  CPF: ${data.pixKeys.cpfKeys.length}`);
  console.log(`  CNPJ: ${data.pixKeys.cnpjKeys.length}`);
  console.log(`  EVP/Random: ${data.pixKeys.randomKeys.length}`);
  console.log('-'.repeat(70));
  console.log(`Erros: ${data.errors.length}`);
  console.log('='.repeat(70));

  if (data.accounts.length > 0 && data.accounts.length <= 10) {
    console.log('\nACCOUNTS CRIADAS:');
    data.accounts.forEach((acc, i) => {
      const maskedDoc = acc.document ? `***${acc.document.slice(-4)}` : 'N/A';
      console.log(`  ${i + 1}. ${acc.id} (${maskedDoc})`);
    });
  }

  if (data.holders.length > 0 && data.holders.length <= 10) {
    console.log('\nHOLDERS CRIADOS:');
    data.holders.forEach((h, i) => {
      const maskedDoc = h.document ? `***${h.document.slice(-4)}` : 'N/A';
      console.log(`  ${i + 1}. ${h.id} - ${h.name} (${maskedDoc})`);
    });
  }

  console.log('\n');
}
