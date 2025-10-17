import { postRequest } from '../utils/fetcher'
import { ORGANIZATION_ID, LEDGER_ID, MIDAZ_BASE_PATH } from '../fixtures/config'
import { ASSETS } from '../fixtures/assets'

interface AssetPayload {
  name: string
  type: string
  code: string
  status: {
    code: string
    description?: string
  }
}

/**
 * Create an asset via the Onboarding API
 */
async function createAsset(payload: AssetPayload) {
  const url = `${MIDAZ_BASE_PATH}/v1/organizations/${ORGANIZATION_ID}/ledgers/${LEDGER_ID}/assets`

  try {
    const response = await postRequest(url, payload)
    return response
  } catch (error) {
    console.error(`Failed to create asset ${payload.code}:`, error)
    throw error
  }
}

/**
 * Setup test assets in the database via API
 * Creates:
 * 1. Real (BRL) - Currency asset
 * 2. Bitcoin (BTC) - Crypto asset
 */
export async function setupAssets() {
  if (!ORGANIZATION_ID) {
    throw new Error('ORGANIZATION_ID environment variable is required')
  }

  if (!LEDGER_ID) {
    throw new Error('LEDGER_ID environment variable is required')
  }

  try {
    // eslint-disable-next-line no-console
    console.log('Creating assets...')

    // Create Currency Asset: Real (BRL)
    const brlAsset = await createAsset(ASSETS.BRL)
    // eslint-disable-next-line no-console
    console.log('✓ Created BRL asset:', brlAsset.id)

    // Create Crypto Asset: Bitcoin (BTC)
    const btcAsset = await createAsset(ASSETS.BTC)
    // eslint-disable-next-line no-console
    console.log('✓ Created BTC asset:', btcAsset.id)

    // eslint-disable-next-line no-console
    console.log('✓ Test assets created successfully')

    return {
      brl: brlAsset,
      btc: btcAsset
    }
  } catch (error) {
    console.error('Failed to setup assets:', error)
    throw error
  }
}

// Run if executed directly
if (require.main === module) {
  setupAssets()
    .then(() => process.exit(0))
    .catch((error) => {
      console.error(error)
      process.exit(1)
    })
}
