#!/usr/bin/env node

const { randomUUID } = require('crypto');

const CRM_BASE_URL = 'http://localhost:4003/v1';
const ORGANIZATION_ID = '0197127c-efde-7dcd-959d-d37ea6d5f7e4'; // Beatty, Bayer and Koelpin
const LEDGER_ID = '0197127c-eff1-7b09-841f-b173aba53042'; // Their ledger

// Sample account IDs from the ledger
const ACCOUNT_IDS = [
  '0197127c-f098-7416-a3bb-17023cc1e92c',
  '0197127c-f09c-7874-954d-01d8e4e15d3f',
  '0197127c-f0a0-7863-ab89-9f956b913a65',
  '0197127c-f0a2-7897-9ad2-e1f87ffd9c81',
  '0197127c-f0a6-78fe-9b1a-c07306b41abe'
];

// Random helpers
const random = {
  pick: (arr) => arr[Math.floor(Math.random() * arr.length)],
  number: (min, max) => Math.floor(Math.random() * (max - min + 1)) + min,
};

// Fetch all holders
async function fetchHolders() {
  try {
    const response = await fetch(`${CRM_BASE_URL}/holders?limit=50`, {
      method: 'GET',
      headers: {
        'Content-Type': 'application/json',
        'X-Organization-Id': ORGANIZATION_ID,
        'x-lerian-id': randomUUID()
      }
    });

    if (response.ok) {
      const data = await response.json();
      return data.items || [];
    } else {
      console.error('Failed to fetch holders');
      return [];
    }
  } catch (error) {
    console.error('Error fetching holders:', error.message);
    return [];
  }
}

// Create alias
async function createAlias(holderId, alias) {
  try {
    const response = await fetch(`${CRM_BASE_URL}/holders/${holderId}/aliases`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Organization-Id': ORGANIZATION_ID,
        'x-lerian-id': randomUUID()
      },
      body: JSON.stringify(alias)
    });

    if (response.ok) {
      const created = await response.json();
      console.log(`  ✅ Created alias for account: ${alias.accountId}`);
      return created;
    } else {
      const error = await response.text();
      console.error(`  ❌ Failed to create alias: ${error}`);
      return null;
    }
  } catch (error) {
    console.error(`  ❌ Error creating alias:`, error.message);
    return null;
  }
}

// Main function
async function main() {
  console.log('🚀 Starting CRM alias generation for existing holders...\n');
  console.log(`Organization: ${ORGANIZATION_ID}`);
  console.log(`Ledger: ${LEDGER_ID}\n`);

  // Fetch all existing holders
  console.log('📋 Fetching existing holders...');
  const holders = await fetchHolders();
  console.log(`Found ${holders.length} holders\n`);

  if (holders.length === 0) {
    console.log('No holders found. Please create holders first.');
    return;
  }

  // Create aliases for each holder
  console.log('🔗 Creating aliases...');
  let totalAliases = 0;
  
  for (const holder of holders) {
    console.log(`\nCreating aliases for ${holder.name} (${holder.type}):`);
    
    // Create 1-3 aliases per holder
    const aliasCount = random.number(1, 3);
    
    for (let i = 0; i < aliasCount; i++) {
      const accountId = random.pick(ACCOUNT_IDS);
      const aliasTypes = ['bank_account', 'pix'];
      const aliasType = random.pick(aliasTypes);
      
      const alias = {
        ledgerId: LEDGER_ID,
        accountId: accountId,
        metadata: {
          source: 'demo_generator',
          aliasType: aliasType,
          holderName: holder.name,
          holderDocument: holder.document,
          holderType: holder.type,
          createdAt: new Date().toISOString()
        }
      };

      // Add banking details for bank account type
      if (aliasType === 'bank_account') {
        const banks = [
          { code: '001', name: 'Banco do Brasil' },
          { code: '237', name: 'Bradesco' },
          { code: '341', name: 'Itaú' },
          { code: '033', name: 'Santander' },
          { code: '104', name: 'Caixa Econômica' }
        ];
        const bank = random.pick(banks);
        
        alias.bankingDetails = {
          bankId: bank.code,
          branch: String(random.number(1000, 9999)),
          account: String(random.number(100000, 99999999)),
          type: 'CACC',
          countryCode: 'BR'
        };
      }

      const createdAlias = await createAlias(holder.id, alias);
      if (createdAlias) totalAliases++;
    }
  }

  // Summary
  console.log('\n\n✨ Alias generation complete!');
  console.log(`   - Total holders: ${holders.length}`);
  console.log(`   - Total aliases created: ${totalAliases}`);
  console.log('\n🎉 You can now view the data in the CRM console at http://localhost:8081/plugins/crm');
}

// Run the script
main().catch(console.error);