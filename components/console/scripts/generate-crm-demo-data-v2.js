#!/usr/bin/env node

const { randomUUID } = require('crypto');

const CRM_BASE_URL = 'http://localhost:4003/v1';
const ORGANIZATION_ID = '019712c0-6335-70af-aa93-24af25e378b0'; // Current organization
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

// Generate individual holders with only supported fields
const individualHolders = [
  {
    type: 'NATURAL_PERSON',
    name: 'João Silva',
    document: '12345678900',
    metadata: {
      source: 'demo_generator',
      customerSince: '2023-01-15'
    }
  },
  {
    type: 'NATURAL_PERSON',
    name: 'Maria Oliveira',
    document: '98765432100',
    metadata: {
      source: 'demo_generator',
      customerSince: '2022-06-20'
    }
  },
  {
    type: 'NATURAL_PERSON',
    name: 'Pedro Santos',
    document: '45678912300',
    metadata: {
      source: 'demo_generator',
      customerSince: '2023-03-10'
    }
  },
  {
    type: 'NATURAL_PERSON',
    name: 'Ana Costa',
    document: '78912345600',
    metadata: {
      source: 'demo_generator',
      customerSince: '2024-01-01'
    }
  },
  {
    type: 'NATURAL_PERSON',
    name: 'Carlos Ferreira',
    document: '32165498700',
    metadata: {
      source: 'demo_generator',
      customerSince: '2023-07-15'
    }
  },
  {
    type: 'NATURAL_PERSON',
    name: 'Fernanda Lima',
    document: '65432198700',
    metadata: {
      source: 'demo_generator',
      customerSince: '2023-09-01'
    }
  },
  {
    type: 'NATURAL_PERSON',
    name: 'Ricardo Alves',
    document: '14725836900',
    metadata: {
      source: 'demo_generator',
      customerSince: '2023-12-01'
    }
  },
  {
    type: 'NATURAL_PERSON',
    name: 'Patricia Mendes',
    document: '25836914700',
    metadata: {
      source: 'demo_generator',
      customerSince: '2024-02-15'
    }
  },
  {
    type: 'NATURAL_PERSON',
    name: 'Lucas Ribeiro',
    document: '36925814700',
    metadata: {
      source: 'demo_generator',
      customerSince: '2024-03-20'
    }
  },
  {
    type: 'NATURAL_PERSON',
    name: 'Juliana Souza',
    document: '85274196300',
    metadata: {
      source: 'demo_generator',
      customerSince: '2024-04-10'
    }
  }
];

// Generate corporate holders
const corporateHolders = [
  {
    type: 'LEGAL_PERSON',
    name: 'Tech Solutions LTDA',
    document: '12345678000190',
    metadata: {
      source: 'demo_generator',
      customerSince: '2021-05-10',
      industry: 'Tecnologia'
    }
  },
  {
    type: 'LEGAL_PERSON',
    name: 'Consultoria Empresarial S.A.',
    document: '98765432000110',
    metadata: {
      source: 'demo_generator',
      customerSince: '2022-02-01',
      industry: 'Consultoria'
    }
  },
  {
    type: 'LEGAL_PERSON',
    name: 'Comércio Digital ME',
    document: '45678901000123',
    metadata: {
      source: 'demo_generator',
      customerSince: '2023-11-20',
      industry: 'E-commerce'
    }
  },
  {
    type: 'LEGAL_PERSON',
    name: 'Logística Express EIRELI',
    document: '78945612000134',
    metadata: {
      source: 'demo_generator',
      customerSince: '2023-08-15',
      industry: 'Logística'
    }
  },
  {
    type: 'LEGAL_PERSON',
    name: 'Indústria Nacional S.A.',
    document: '32178945000156',
    metadata: {
      source: 'demo_generator',
      customerSince: '2022-12-01',
      industry: 'Indústria'
    }
  }
];

// Create holder
async function createHolder(holder) {
  try {
    const response = await fetch(`${CRM_BASE_URL}/holders`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Organization-Id': ORGANIZATION_ID,
        'x-lerian-id': randomUUID()
      },
      body: JSON.stringify(holder)
    });

    if (response.ok) {
      const created = await response.json();
      console.log(`✅ Created holder: ${holder.name}`);
      return created;
    } else {
      const error = await response.text();
      console.error(`❌ Failed to create holder ${holder.name}: ${error}`);
      return null;
    }
  } catch (error) {
    console.error(`❌ Error creating holder ${holder.name}:`, error.message);
    return null;
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
  console.log('🚀 Starting CRM demo data generation...\n');
  console.log(`Organization: ${ORGANIZATION_ID}`);
  console.log(`Ledger: ${LEDGER_ID}\n`);

  const allHolders = [];
  
  // Create individual holders
  console.log('👤 Creating individual holders...');
  for (const holder of individualHolders) {
    const created = await createHolder(holder);
    if (created) allHolders.push(created);
  }
  console.log(`Created ${allHolders.filter(h => h.type === 'NATURAL_PERSON').length} individual holders\n`);

  // Create corporate holders
  console.log('🏢 Creating corporate holders...');
  for (const holder of corporateHolders) {
    const created = await createHolder(holder);
    if (created) allHolders.push(created);
  }
  console.log(`Created ${allHolders.filter(h => h.type === 'LEGAL_PERSON').length} corporate holders\n`);

  // Create aliases for each holder
  console.log('🔗 Creating aliases...');
  let totalAliases = 0;
  
  for (const holder of allHolders) {
    console.log(`\nCreating aliases for ${holder.name}:`);
    
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
  console.log('\n\n✨ Demo data generation complete!');
  console.log(`   - Individual holders: ${allHolders.filter(h => h.type === 'NATURAL_PERSON').length}`);
  console.log(`   - Corporate holders: ${allHolders.filter(h => h.type === 'LEGAL_PERSON').length}`);
  console.log(`   - Total aliases: ${totalAliases}`);
  console.log('\n🎉 You can now view the data in the CRM console at http://localhost:8081/plugins/crm');
}

// Run the script
main().catch(console.error);