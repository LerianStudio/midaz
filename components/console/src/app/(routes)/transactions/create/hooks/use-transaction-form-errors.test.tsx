import { act, renderHook } from '@testing-library/react'
import { useTransactionFormErrors } from './use-transaction-form-errors'
import { TransactionMode } from './use-transaction-mode'
import { externalAccountAliasPrefix } from '@/core/infrastructure/midaz/config/config'

jest.mock('react-intl', () => {
  const reactIntl = jest.requireActual('react-intl')
  const intl = reactIntl.createIntl({
    locale: 'en'
  })

  return {
    ...reactIntl,
    useIntl: () => intl
  }
})

jest.mock('./use-transaction-mode', () => ({
  TransactionMode: {
    SIMPLE: 'simple',
    COMPLEX: 'complex'
  },
  useTransactionMode: jest.fn().mockReturnValue({
    mode: 'complex'
  })
}))

describe('useTransactionFormErrors', () => {
  it('should return no errors when all fields are valid', () => {
    const validFormData = {
      value: '',
      source: [],
      destination: []
    }

    const { result } = renderHook(() =>
      useTransactionFormErrors(validFormData as any, {})
    )

    expect(result.current.errors).toEqual({})
  })

  it('should return a error if total debit amount total is not equal to transaction value', () => {
    const formData = {
      value: 100,
      source: [
        {
          account: 'account1',
          value: 50
        }
      ],
      destination: []
    }

    const { result } = renderHook(() =>
      useTransactionFormErrors(formData as any, {})
    )

    expect(result.current.errors['debit']).toBeDefined()
  })

  it('should return a error if total credit amount total is not equal to transaction value', () => {
    const formData = {
      value: 100,
      source: [],
      destination: [
        {
          account: 'account2',
          value: 50
        }
      ]
    }

    const { result } = renderHook(() =>
      useTransactionFormErrors(formData as any, {})
    )

    expect(result.current.errors['credit']).toBeDefined()
  })

  it('should ignore external accounts when checking for insufficient funds', () => {
    const formData = {
      value: 100,
      asset: 'BRL',
      source: [
        {
          account: `${externalAccountAliasPrefix}BRL`,
          value: 100
        }
      ],
      destination: [
        {
          account: 'account1',
          value: 100
        }
      ]
    }
    const accounts = {
      [`${externalAccountAliasPrefix}BRL`]: {
        balances: [
          {
            assetCode: 'BRL',
            available: 50
          }
        ]
      }
    }

    const { result } = renderHook(() =>
      useTransactionFormErrors(formData as any, accounts as any)
    )
    act(() => {
      result.current.validate()
    })
    expect(result.current.errors).toEqual({})
  })

  it('should detect if data loss is prone to happen', () => {
    const formData = {
      value: 100,
      asset: 'BRL',
      source: [
        {
          account: 'account1',
          value: 100
        },
        {
          account: 'account2',
          value: 100
        }
      ],
      destination: [
        {
          account: 'account3',
          value: 100
        }
      ]
    }

    const { result } = renderHook(() =>
      useTransactionFormErrors(formData as any, {})
    )
    act(() => {
      result.current.validate()
    })
    expect(result.current.errors['data-loss']).toBeDefined()
  })
})
