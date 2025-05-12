import {
  parseCreateData,
  parseUpdateData,
  OrganizationFormData
} from './organizations-form'
import { omit } from 'lodash'

describe('parseCreateData', () => {
  it('should correctly parse the input data', () => {
    const input: OrganizationFormData = {
      id: '1',
      parentOrganizationId: '2',
      legalName: 'Test Org',
      doingBusinessAs: 'Test DBA',
      legalDocument: 'Doc123',
      address: {
        line1: '123 Test St',
        line2: 'Apt 4',
        country: 'Testland',
        state: 'TS',
        city: 'Test City',
        zipCode: '12345'
      },
      metadata: {
        key1: 'value1'
      },
      avatar: 'avatar.png'
    }

    const expectedOutput = {
      ...input
    }

    expect(parseCreateData(input)).toEqual(expectedOutput)
  })
})

describe('parseUpdateData', () => {
  it('should correctly parse the input data, omitting id and legalDocument', () => {
    const input: OrganizationFormData = {
      id: '1',
      parentOrganizationId: '2',
      legalName: 'Test Org',
      doingBusinessAs: 'Test DBA',
      legalDocument: 'Doc123',
      address: {
        line1: '123 Test St',
        line2: 'Apt 4',
        country: 'Testland',
        state: 'TS',
        city: 'Test City',
        zipCode: '12345'
      },
      metadata: {
        key1: 'value1'
      },
      accentColor: '#FF0000',
      avatar: 'avatar.png'
    }

    const expectedOutput = {
      ...omit(input, ['id', 'legalDocument']),
      metadata: {
        ...input.metadata
      }
    }

    expect(parseUpdateData(input)).toEqual(expectedOutput)
  })
})
