'use server'

import { createHolder } from './crm'
import { CreateHolderEntity } from '@/core/domain/entities/holder-entity'
import { faker } from '@faker-js/faker/locale/pt_BR'

export async function generateCRMDemoData(organizationId: string = 'default') {
  const results = {
    holders: [] as any[],
    errors: [] as any[]
  }

  try {
    // Generate 10 individual customers
    for (let i = 0; i < 10; i++) {
      const holder: CreateHolderEntity = {
        type: 'NATURAL_PERSON',
        name: faker.person.fullName(),
        document: faker.helpers.regexpStyleStringParse('[0-9]{3}.[0-9]{3}.[0-9]{3}-[0-9]{2}'), // CPF format
        nationality: 'BR',
        email: faker.internet.email(),
        phoneNumber: faker.phone.number('+55 11 9####-####'),
        address: {
          line1: faker.location.streetAddress(),
          city: faker.location.city(),
          state: faker.location.state({ abbreviated: true }),
          postalCode: faker.location.zipCode('#####-###'),
          country: 'BR'
        },
        metadata: {
          source: 'demo_generator',
          createdAt: new Date().toISOString()
        }
      }

      try {
        const result = await createHolder(holder)
        if (result.success && result.data) {
          results.holders.push(result.data)
        } else {
          results.errors.push({ holder, error: result.error })
        }
      } catch (error) {
        results.errors.push({ holder, error })
      }
    }

    // Generate 5 corporate customers
    for (let i = 0; i < 5; i++) {
      const holder: CreateHolderEntity = {
        type: 'LEGAL_PERSON',
        name: faker.company.name(),
        document: faker.helpers.regexpStyleStringParse('[0-9]{2}.[0-9]{3}.[0-9]{3}/[0-9]{4}-[0-9]{2}'), // CNPJ format
        nationality: 'BR',
        email: faker.internet.email(),
        phoneNumber: faker.phone.number('+55 11 ####-####'),
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
          industry: faker.company.buzzNoun(),
          createdAt: new Date().toISOString()
        }
      }

      try {
        const result = await createHolder(holder)
        if (result.success && result.data) {
          results.holders.push(result.data)
        } else {
          results.errors.push({ holder, error: result.error })
        }
      } catch (error) {
        results.errors.push({ holder, error })
      }
    }

    return {
      success: true,
      data: results,
      message: `Generated ${results.holders.length} holders with ${results.errors.length} errors`
    }
  } catch (error) {
    return {
      success: false,
      error: error instanceof Error ? error.message : 'Failed to generate demo data'
    }
  }
}