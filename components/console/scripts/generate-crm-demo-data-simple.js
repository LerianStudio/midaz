#!/usr/bin/env node

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
  cpf: () => {
    const n = () => Math.floor(Math.random() * 10);
    return `${n()}${n()}${n()}.${n()}${n()}${n()}.${n()}${n()}${n()}-${n()}${n()}`;
  },
  cnpj: () => {
    const n = () => Math.floor(Math.random() * 10);
    return `${n()}${n()}.${n()}${n()}${n()}.${n()}${n()}${n()}/0001-${n()}${n()}`;
  },
  phone: () => {
    const ddd = random.pick(['11', '21', '31', '41', '51', '61', '71', '81', '91']);
    const prefix = random.pick(['9', '8']);
    return `+55 ${ddd} ${prefix}${random.number(1000, 9999)}-${random.number(1000, 9999)}`;
  }
};

// Sample data
const firstNames = ['João', 'Maria', 'Pedro', 'Ana', 'Carlos', 'Fernanda', 'Lucas', 'Juliana', 'Bruno', 'Patricia'];
const lastNames = ['Silva', 'Santos', 'Oliveira', 'Souza', 'Rodrigues', 'Ferreira', 'Alves', 'Pereira', 'Lima', 'Costa'];
const companies = ['Tech', 'Solutions', 'Consultoria', 'Serviços', 'Comércio', 'Indústria', 'Logística', 'Digital', 'Global', 'Express'];
const companyTypes = ['LTDA', 'S.A.', 'EIRELI', 'ME', 'EPP'];
const streets = ['Rua das Flores', 'Av. Paulista', 'Rua XV de Novembro', 'Av. Brasil', 'Rua Augusta'];
const cities = ['São Paulo', 'Rio de Janeiro', 'Belo Horizonte', 'Curitiba', 'Porto Alegre', 'Salvador', 'Brasília'];
const states = ['SP', 'RJ', 'MG', 'PR', 'RS', 'BA', 'DF'];

// Generate individual holders
const individualHolders = [
  {
    type: 'NATURAL_PERSON',
    name: 'João Silva',
    document: '123.456.789-00',
    contacts: [
      {
        type: 'email',
        value: 'joao.silva@email.com',
        isPrimary: true
      },
      {
        type: 'phone',
        value: '+55 11 98765-4321',
        isPrimary: true
      }
    ],
    metadata: {
      source: 'demo_generator',
      customerSince: '2023-01-15',
      preferredLanguage: 'pt-BR',
      occupation: 'Engenheiro de Software'
    }
  },
  {
    type: 'NATURAL_PERSON',
    name: 'Maria Oliveira',
    document: '987.654.321-00',
    nationality: 'BR',
    email: 'maria.oliveira@email.com',
    phoneNumber: '+55 21 91234-5678',
    address: {
      line1: 'Av. Copacabana, 500',
      line2: 'Cobertura',
      city: 'Rio de Janeiro',
      state: 'RJ',
      postalCode: '22020-001',
      country: 'BR'
    },
    metadata: {
      source: 'demo_generator',
      customerSince: '2022-06-20',
      preferredLanguage: 'pt-BR',
      occupation: 'Médica'
    }
  },
  {
    type: 'NATURAL_PERSON',
    name: 'Pedro Santos',
    document: '456.789.123-00',
    nationality: 'BR',
    email: 'pedro.santos@email.com',
    phoneNumber: '+55 31 99876-5432',
    address: {
      line1: 'Rua da Bahia, 789',
      city: 'Belo Horizonte',
      state: 'MG',
      postalCode: '30123-456',
      country: 'BR'
    },
    metadata: {
      source: 'demo_generator',
      customerSince: '2023-03-10',
      preferredLanguage: 'pt-BR',
      occupation: 'Advogado'
    }
  },
  {
    type: 'NATURAL_PERSON',
    name: 'Ana Costa',
    document: '789.123.456-00',
    nationality: 'BR',
    email: 'ana.costa@email.com',
    phoneNumber: '+55 11 94567-8901',
    address: {
      line1: 'Alameda Santos, 1000',
      line2: 'Sala 1500',
      city: 'São Paulo',
      state: 'SP',
      postalCode: '01419-000',
      country: 'BR'
    },
    metadata: {
      source: 'demo_generator',
      customerSince: '2024-01-01',
      preferredLanguage: 'pt-BR',
      occupation: 'Empresária'
    }
  },
  {
    type: 'NATURAL_PERSON',
    name: 'Carlos Ferreira',
    document: '321.654.987-00',
    nationality: 'BR',
    email: 'carlos.ferreira@email.com',
    phoneNumber: '+55 41 98765-1234',
    address: {
      line1: 'Rua XV de Novembro, 200',
      city: 'Curitiba',
      state: 'PR',
      postalCode: '80020-300',
      country: 'BR'
    },
    metadata: {
      source: 'demo_generator',
      customerSince: '2023-07-15',
      preferredLanguage: 'pt-BR',
      occupation: 'Professor'
    }
  }
];

// Generate corporate holders
const corporateHolders = [
  {
    type: 'LEGAL_PERSON',
    name: 'Tech Solutions LTDA',
    document: '12.345.678/0001-90',
    nationality: 'BR',
    email: 'contato@techsolutions.com.br',
    phoneNumber: '+55 11 3333-4444',
    address: {
      line1: 'Av. Faria Lima, 3477',
      line2: '15º andar',
      city: 'São Paulo',
      state: 'SP',
      postalCode: '04538-133',
      country: 'BR'
    },
    metadata: {
      source: 'demo_generator',
      customerSince: '2021-05-10',
      industry: 'Tecnologia',
      employeeCount: '50-200',
      annualRevenue: '5M-10M'
    }
  },
  {
    type: 'LEGAL_PERSON',
    name: 'Consultoria Empresarial S.A.',
    document: '98.765.432/0001-10',
    nationality: 'BR',
    email: 'contato@consultoria.com.br',
    phoneNumber: '+55 21 2222-3333',
    address: {
      line1: 'Av. Rio Branco, 1',
      line2: '20º andar',
      city: 'Rio de Janeiro',
      state: 'RJ',
      postalCode: '20090-003',
      country: 'BR'
    },
    metadata: {
      source: 'demo_generator',
      customerSince: '2022-02-01',
      industry: 'Consultoria',
      employeeCount: '10-50',
      annualRevenue: '1M-5M'
    }
  },
  {
    type: 'LEGAL_PERSON',
    name: 'Comércio Digital ME',
    document: '45.678.901/0001-23',
    nationality: 'BR',
    email: 'vendas@comerciodigital.com.br',
    phoneNumber: '+55 11 5555-6666',
    address: {
      line1: 'Rua Augusta, 2500',
      city: 'São Paulo',
      state: 'SP',
      postalCode: '01412-100',
      country: 'BR'
    },
    metadata: {
      source: 'demo_generator',
      customerSince: '2023-11-20',
      industry: 'E-commerce',
      employeeCount: '1-10',
      annualRevenue: '500k-1M'
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
        'X-Organization-Id': ORGANIZATION_ID
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
        'X-Organization-Id': ORGANIZATION_ID
      },
      body: JSON.stringify(alias)
    });

    if (response.ok) {
      const created = await response.json();
      console.log(`  ✅ Created alias: ${alias.name}`);
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
        name: `${holder.name} - ${random.pick(['Conta Principal', 'Conta Corrente', 'Conta Poupança'])}`,
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
          bankCode: random.pick(['001', '237', '341', '033', '104']), // Brazilian bank codes
          branch: String(random.number(1000, 9999)),
          number: String(random.number(100000, 99999999)),
          type: random.pick(['checking', 'savings'])
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