import {
  getFetcher,
  postFetcher,
  patchFetcher,
  deleteFetcher,
  serverFetcher
} from './index'

global.fetch = jest.fn(() =>
  Promise.resolve({
    ok: true,
    json: () => Promise.resolve({ data: 'test' })
  })
) as jest.Mock

jest.mock('next/navigation', () => ({
  redirect: jest.fn()
}))

describe('fetcher functions', () => {
  beforeEach(() => {
    ;(fetch as jest.Mock).mockClear()
  })

  test('getFetcher should handle successful response', async () => {
    const fetcher = getFetcher('/test-url')
    const response = await fetcher()
    expect(response).toEqual({ data: 'test' })
    expect(fetch).toHaveBeenCalledWith('/test-url', {
      method: 'GET',
      headers: {
        'Content-Type': 'application/json'
      }
    })
  })

  test('postFetcher should handle successful response', async () => {
    const fetcher = postFetcher('/test-url')
    const response = await fetcher({ key: 'value' })
    expect(response).toEqual({ data: 'test' })
    expect(fetch).toHaveBeenCalledWith('/test-url', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({ key: 'value' })
    })
  })

  test('patchFetcher should handle successful response', async () => {
    const fetcher = patchFetcher('/test-url')
    const response = await fetcher({ key: 'value' })
    expect(response).toEqual({ data: 'test' })
    expect(fetch).toHaveBeenCalledWith('/test-url', {
      method: 'PATCH',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({ key: 'value' })
    })
  })

  test('deleteFetcher should handle successful response', async () => {
    const fetcher = deleteFetcher('/test-url')
    const response = await fetcher({ id: '123' })
    expect(response).toEqual({ data: 'test' })
    expect(fetch).toHaveBeenCalledWith('/test-url/123', {
      method: 'DELETE',
      headers: {
        'Content-Type': 'application/json'
      }
    })
  })

  test('serverFetcher should handle successful action', async () => {
    const action = jest.fn().mockResolvedValue('action result')
    const result = await serverFetcher(action)
    expect(result).toBe('action result')
    expect(action).toHaveBeenCalled()
  })

  test('serverFetcher should handle failed action and redirect', async () => {
    const action = jest.fn().mockRejectedValue(new Error('test error'))
    const result = await serverFetcher(action)
    expect(result).toBeNull()
  })
})
