#!/usr/bin/env node

const { randomUUID } = require('crypto');

const CRM_BASE_URL = 'http://localhost:4003/v1';
const ONBOARDING_BASE_URL = 'http://localhost:3000/v1';

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
  }
];

// Fetch all organizations
async function fetchOrganizations() {
  try {
    const response = await fetch(`${ONBOARDING_BASE_URL}/organizations`, {
      method: 'GET',
      headers: {
        'Content-Type': 'application/json'
      }
    });

    if (response.ok) {
      const data = await response.json();
      return data.items || [];
    } else {
      console.error('Failed to fetch organizations');
      return [];
    }
  } catch (error) {
    console.error('Error fetching organizations:', error.message);
    return [];
  }
}

// Fetch ledgers for an organization
async function fetchLedgers(organizationId) {
  try {
    const response = await fetch(
      `${ONBOARDING_BASE_URL}/organizations/${organizationId}/ledgers`,
      {
        method: 'GET',
        headers: {
          'Content-Type': 'application/json'
        }
      }
    );

    if (response.ok) {
      const data = await response.json();
      return data.items || [];
    } else {
      return [];
    }
  } catch (error) {
    console.error(`Error fetching ledgers for org ${organizationId}:`, error.message);
    return [];
  }
}

// Fetch accounts for a ledger
async function fetchAccounts(organizationId, ledgerId) {
  try {
    const response = await fetch(
      `${ONBOARDING_BASE_URL}/organizations/${organizationId}/ledgers/${ledgerId}/accounts?limit=20`,
      {
        method: 'GET',
        headers: {
          'Content-Type': 'application/json'
        }
      }
    );

    if (response.ok) {
      const data = await response.json();
      return (data.items || []).map(acc => acc.id);
    } else {
      return [];
    }
  } catch (error) {
    console.error(`Error fetching accounts for ledger ${ledgerId}:`, error.message);
    return [];
  }
}

// Create holder
async function createHolder(organizationId, holder) {
  try {
    const response = await fetch(`${CRM_BASE_URL}/holders`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Organization-Id': organizationId,
        'x-lerian-id': randomUUID()
      },
      body: JSON.stringify(holder)
    });

    if (response.ok) {
      const created = await response.json();
      console.log(`    ✅ Created holder: ${holder.name}`);
      return created;
    } else {
      const error = await response.text();
      
      // Check if holder already exists (document already in use)
      if (error.includes('CRM-0010') || error.includes('document can only be associated')) {
        console.log(`    ⚠️  Holder already exists: ${holder.name}`);
        // Try to fetch existing holder
        return await fetchHolderByDocument(organizationId, holder.document);
      }
      
      console.error(`    ❌ Failed to create holder ${holder.name}: ${error}`);
      return null;
    }
  } catch (error) {
    console.error(`    ❌ Error creating holder ${holder.name}:`, error.message);
    return null;
  }
}

// Fetch holder by document
async function fetchHolderByDocument(organizationId, document) {
  try {
    const response = await fetch(`${CRM_BASE_URL}/holders?limit=100`, {
      method: 'GET',
      headers: {
        'Content-Type': 'application/json',
        'X-Organization-Id': organizationId,
        'x-lerian-id': randomUUID()
      }
    });

    if (response.ok) {
      const data = await response.json();
      const holder = (data.items || []).find(h => h.document === document);
      return holder || null;
    }
    return null;
  } catch (error) {
    return null;
  }
}

// Create alias
async function createAlias(organizationId, holderId, alias) {
  try {
    const response = await fetch(`${CRM_BASE_URL}/holders/${holderId}/aliases`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Organization-Id': organizationId,
        'x-lerian-id': randomUUID()
      },
      body: JSON.stringify(alias)
    });

    if (response.ok) {
      const created = await response.json();
      console.log(`      ✅ Created alias for account: ${alias.accountId.slice(-8)}`);
      return created;
    } else {
      const error = await response.text();
      
      // Skip if account already associated
      if (error.includes('CRM-0013') || error.includes('already associated')) {
        console.log(`      ⚠️  Account already linked: ${alias.accountId.slice(-8)}`);
        return null;
      }
      
      console.error(`      ❌ Failed to create alias: ${error}`);
      return null;
    }
  } catch (error) {
    console.error(`      ❌ Error creating alias:`, error.message);
    return null;
  }
}

// Main function
async function main() {
  console.log('🚀 Starting CRM demo data generation for ALL organizations...\n');

  // Fetch all organizations
  console.log('📋 Fetching all organizations...');
  const organizations = await fetchOrganizations();
  console.log(`Found ${organizations.length} organizations\n`);

  if (organizations.length === 0) {
    console.log('❌ No organizations found. Please create organizations first.');
    return;
  }

  // Process each organization
  for (const org of organizations) {
    console.log(`\n═══════════════════════════════════════════════════`);
    console.log(`🏢 Organization: ${org.legalName || org.name}`);
    console.log(`   ID: ${org.id}`);
    console.log(`═══════════════════════════════════════════════════\n`);

    // Fetch ledgers for this organization
    const ledgers = await fetchLedgers(org.id);
    console.log(`  📚 Found ${ledgers.length} ledgers`);

    if (ledgers.length === 0) {
      console.log('  ⚠️  No ledgers found for this organization, skipping...');
      continue;
    }

    // Create holders for this organization
    console.log('\n  👤 Creating holders...');
    const allHolders = [];
    
    // Create individual holders (sample subset)
    for (const holder of individualHolders.slice(0, 3)) {
      const created = await createHolder(org.id, {
        ...holder,
        // Make document unique per org by adding org suffix
        document: holder.document + org.id.slice(-4)
      });
      if (created) allHolders.push(created);
    }
    
    // Create corporate holders (sample subset)
    for (const holder of corporateHolders.slice(0, 2)) {
      const created = await createHolder(org.id, {
        ...holder,
        // Make document unique per org by adding org suffix
        document: holder.document + org.id.slice(-4)
      });
      if (created) allHolders.push(created);
    }

    console.log(`  ✨ Total holders: ${allHolders.length}`);

    // Process each ledger
    for (const ledger of ledgers) {
      console.log(`\n  📗 Ledger: ${ledger.name} (${ledger.id})`);
      
      // Fetch accounts for this ledger
      const accountIds = await fetchAccounts(org.id, ledger.id);
      console.log(`     Found ${accountIds.length} accounts`);

      if (accountIds.length === 0) {
        console.log('     ⚠️  No accounts found for this ledger, skipping aliases...');
        continue;
      }

      // Create aliases for some holders
      console.log('     🔗 Creating aliases...');
      let totalAliases = 0;
      
      // Only create aliases for first few holders to avoid too much data
      const holdersToProcess = allHolders.slice(0, Math.min(3, allHolders.length));
      
      for (const holder of holdersToProcess) {
        console.log(`\n     For ${holder.name}:`);
        
        // Create 1-2 aliases per holder
        const aliasCount = random.number(1, 2);
        const availableAccounts = [...accountIds];
        
        for (let i = 0; i < aliasCount && availableAccounts.length > 0; i++) {
          // Pick a random account
          const accountIndex = random.number(0, availableAccounts.length - 1);
          const accountId = availableAccounts[accountIndex];
          availableAccounts.splice(accountIndex, 1); // Remove used account
          
          const aliasTypes = ['bank_account', 'pix'];
          const aliasType = random.pick(aliasTypes);
          
          const alias = {
            ledgerId: ledger.id,
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

          const createdAlias = await createAlias(org.id, holder.id, alias);
          if (createdAlias) totalAliases++;
        }
      }
      
      console.log(`\n     ✨ Total aliases created for this ledger: ${totalAliases}`);
    }
  }

  // Summary
  console.log('\n\n════════════════════════════════════════════════════');
  console.log('✨ Demo data generation complete for all organizations!');
  console.log('🎉 You can now view the CRM data in the console');
  console.log('   Navigate to: http://localhost:8081/plugins/crm/customers');
  console.log('════════════════════════════════════════════════════\n');
}

// Run the script
main().catch(console.error);