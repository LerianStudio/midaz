#!/usr/bin/env node

const { randomUUID } = require('crypto');

const CRM_BASE_URL = 'http://localhost:4003/v1';
const ORGANIZATION_ID = '019712c0-6335-70af-aa93-24af25e378b0';

async function checkAliases() {
  try {
    // First fetch holders
    console.log('Fetching holders...');
    const holdersResponse = await fetch(`${CRM_BASE_URL}/holders?limit=10`, {
      headers: {
        'X-Organization-Id': ORGANIZATION_ID,
        'x-lerian-id': randomUUID()
      }
    });

    const holdersData = await holdersResponse.json();
    console.log(`Found ${holdersData.items.length} holders`);

    // Check aliases for each holder
    for (const holder of holdersData.items) {
      console.log(`\nChecking aliases for ${holder.name} (${holder.id})`);
      
      const aliasesResponse = await fetch(`${CRM_BASE_URL}/holders/${holder.id}/aliases`, {
        headers: {
          'X-Organization-Id': ORGANIZATION_ID,
          'x-lerian-id': randomUUID()
        }
      });

      const aliasesData = await aliasesResponse.json();
      console.log(`  Found ${aliasesData.items.length} aliases`);
      
      for (const alias of aliasesData.items) {
        console.log(`    - Account: ${alias.accountId.slice(-8)}, Type: ${alias.metadata?.aliasType || 'unknown'}`);
      }
    }
  } catch (error) {
    console.error('Error:', error);
  }
}

checkAliases();