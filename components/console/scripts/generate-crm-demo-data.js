#!/usr/bin/env node

const { fakerPT_BR: faker } = require('@faker-js/faker');

const CRM_BASE_URL = 'http://localhost:4003/v1';
const ORGANIZATION_ID = '0197127c-efde-7dcd-959d-d37ea6d5f7e4'; // Beatty, Bayer and Koelpin
const LEDGER_ID = '0197127c-eff1-7b09-841f-b173aba53042'; // Their ledger

// Sample account IDs from the ledger (we'll need to map these)
const ACCOUNT_IDS = [
  '0197127c-f098-7416-a3bb-17023cc1e92c',
  '0197127c-f09c-7874-954d-01d8e4e15d3f',
  '0197127c-f0a0-7863-ab89-9f956b913a65',
  '0197127c-f0a2-7897-9ad2-e1f87ffd9c81',
  '0197127c-f0a6-78fe-9b1a-c07306b41abe'
];

// Helper to generate valid CPF
function generateCPF() {
  const num = () => Math.floor(Math.random() * 10);
  return `${num()}${num()}${num()}.${num()}${num()}${num()}.${num()}${num()}${num()}-${num()}${num()}`;
}

// Helper to generate valid CNPJ
function generateCNPJ() {
  const num = () => Math.floor(Math.random() * 10);
  return `${num()}${num()}.${num()}${num()}${num()}.${num()}${num()}${num()}/0001-${num()}${num()}`;
}

// Generate Brazilian phone number
function generatePhone() {
  const ddd = faker.helpers.arrayElement(['11', '21', '31', '41', '51', '61', '71', '81', '91']);
  const prefix = faker.helpers.arrayElement(['9', '8']);
  return `+55 ${ddd} ${prefix}${faker.string.numeric(4)}-${faker.string.numeric(4)}`;
}

// Generate individual holders
async function generateIndividualHolders(count = 10) {
  const holders = [];
  
  for (let i = 0; i < count; i++) {
    const holder = {
      type: 'NATURAL_PERSON',
      name: faker.person.fullName(),
      document: generateCPF(),
      nationality: 'BR',
      email: faker.internet.email(),
      phoneNumber: generatePhone(),
      address: {
        line1: faker.location.streetAddress(),
        line2: faker.helpers.maybe(() => `Apt ${faker.string.numeric(3)}`),
        city: faker.location.city(),
        state: faker.location.state({ abbreviated: true }),
        postalCode: faker.location.zipCode('#####-###'),
        country: 'BR'
      },
      metadata: {
        source: 'demo_generator',
        customerSince: faker.date.past({ years: 3 }).toISOString(),
        preferredLanguage: 'pt-BR',
        occupation: faker.person.jobTitle(),
        monthlyIncome: faker.helpers.arrayElement(['1000-3000', '3000-5000', '5000-10000', '10000+'])
      }
    };

    try {
      const response = await fetch(`${CRM_BASE_URL}/holders`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-Organization-Id': ORGANIZATION_ID
        },
        body: JSON.stringify(holder)
      });

      if (response.ok) {
        const created = await response.json();
        holders.push(created);
        console.log(`✅ Created individual holder: ${holder.name}`);
      } else {
        console.error(`❌ Failed to create holder ${holder.name}:`, await response.text());
      }
    } catch (error) {
      console.error(`❌ Error creating holder ${holder.name}:`, error.message);
    }
  }

  return holders;
}

// Generate corporate holders
async function generateCorporateHolders(count = 5) {
  const holders = [];
  
  for (let i = 0; i < count; i++) {
    const holder = {
      type: 'LEGAL_PERSON',
      name: faker.company.name(),
      document: generateCNPJ(),
      nationality: 'BR',
      email: faker.internet.email({ provider: 'company.com.br' }),
      phoneNumber: generatePhone(),
      address: {
        line1: faker.location.streetAddress(),
        line2: `${faker.location.secondaryAddress()}, ${faker.location.buildingNumber()}`,
        city: faker.location.city(),
        state: faker.location.state({ abbreviated: true }),
        postalCode: faker.location.zipCode('#####-###'),
        country: 'BR'
      },
      metadata: {
        source: 'demo_generator',
        customerSince: faker.date.past({ years: 5 }).toISOString(),
        industry: faker.company.buzzNoun(),
        employeeCount: faker.helpers.arrayElement(['1-10', '11-50', '51-200', '200+']),
        annualRevenue: faker.helpers.arrayElement(['100k-500k', '500k-1M', '1M-5M', '5M+'])
      }
    };

    try {
      const response = await fetch(`${CRM_BASE_URL}/holders`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-Organization-Id': ORGANIZATION_ID
        },
        body: JSON.stringify(holder)
      });

      if (response.ok) {
        const created = await response.json();
        holders.push(created);
        console.log(`✅ Created corporate holder: ${holder.name}`);
      } else {
        console.error(`❌ Failed to create holder ${holder.name}:`, await response.text());
      }
    } catch (error) {
      console.error(`❌ Error creating holder ${holder.name}:`, error.message);
    }
  }

  return holders;
}

// Generate aliases for holders
async function generateAliases(holders) {
  const aliases = [];
  
  for (const holder of holders) {
    // Each holder gets 1-3 aliases
    const aliasCount = faker.number.int({ min: 1, max: 3 });
    
    for (let i = 0; i < aliasCount; i++) {
      const accountId = faker.helpers.arrayElement(ACCOUNT_IDS);
      const aliasType = faker.helpers.arrayElement(['bank_account', 'pix', 'bank_account']);
      
      const alias = {
        name: `${holder.name} - ${faker.helpers.arrayElement(['Conta Principal', 'Conta Secundária', 'Conta Poupança', 'Conta Corrente'])}`,
        ledgerId: LEDGER_ID,
        accountId: accountId,
        type: aliasType,
        metadata: {
          source: 'demo_generator',
          createdAt: new Date().toISOString()
        }
      };

      // Add banking details for bank account type
      if (aliasType === 'bank_account') {
        alias.bankingDetails = {
          bankCode: faker.helpers.arrayElement(['001', '237', '341', '033', '104']), // Brazilian bank codes
          branch: faker.string.numeric(4),
          number: faker.string.numeric({ length: { min: 6, max: 8 } }),
          type: faker.helpers.arrayElement(['checking', 'savings'])
        };
      }

      try {
        const response = await fetch(`${CRM_BASE_URL}/holders/${holder.id}/aliases`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'X-Organization-Id': ORGANIZATION_ID
          },
          body: JSON.stringify(alias)
        });

        if (response.ok) {
          const created = await response.json();
          aliases.push(created);
          console.log(`  ✅ Created alias: ${alias.name}`);
        } else {
          console.error(`  ❌ Failed to create alias for ${holder.name}:`, await response.text());
        }
      } catch (error) {
        console.error(`  ❌ Error creating alias for ${holder.name}:`, error.message);
      }
    }
  }

  return aliases;
}

// Main function
async function main() {
  console.log('🚀 Starting CRM demo data generation...\n');
  console.log(`Organization: ${ORGANIZATION_ID}`);
  console.log(`Ledger: ${LEDGER_ID}\n`);

  try {
    // Generate individual holders
    console.log('👤 Generating individual holders...');
    const individualHolders = await generateIndividualHolders(10);
    console.log(`Created ${individualHolders.length} individual holders\n`);

    // Generate corporate holders
    console.log('🏢 Generating corporate holders...');
    const corporateHolders = await generateCorporateHolders(5);
    console.log(`Created ${corporateHolders.length} corporate holders\n`);

    // Generate aliases
    const allHolders = [...individualHolders, ...corporateHolders];
    console.log('🔗 Generating aliases...');
    const aliases = await generateAliases(allHolders);
    console.log(`Created ${aliases.length} aliases\n`);

    // Summary
    console.log('✨ Demo data generation complete!');
    console.log(`   - Individual holders: ${individualHolders.length}`);
    console.log(`   - Corporate holders: ${corporateHolders.length}`);
    console.log(`   - Total aliases: ${aliases.length}`);
    console.log('\n🎉 You can now view the data in the CRM console!');

  } catch (error) {
    console.error('❌ Fatal error:', error);
    process.exit(1);
  }
}

// Run the script
main().catch(console.error);