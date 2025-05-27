/**
 * TODO: Better error handling
 */
import { MidazApiException } from '@/core/infrastructure/midaz/exceptions/midaz-exceptions'
import { signOut } from 'next-auth/react'
import { redirect } from 'next/navigation'
import { createQueryString } from '../search'

export const getFetcher = (url: string) => {
  return async () => {
    const response = await fetch(url, {
      method: 'GET',
      headers: {
        'Content-Type': 'application/json'
      }
    })

    return responseHandler(response)
  }
}

export const getPaginatedFetcher = (url: string, params?: {}) => {
  return async () => {
    const response = await fetch(url + createQueryString(params), {
      method: 'GET',
      headers: {
        'Content-Type': 'application/json'
      }
    })

    return responseHandler(response)
  }
}

export const postFetcher = (url: string) => {
  return async (body: any) => {
    const response = await fetch(url, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(body)
    })

    return responseHandler(response)
  }
}

export const patchFetcher = (url: string) => {
  return async (body: any) => {
    const response = await fetch(url, {
      method: 'PATCH',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(body)
    })

    return responseHandler(response)
  }
}

export const deleteFetcher = (url: string) => {
  return async ({ id }: { id: string }) => {
    const response = await fetch(`${url}/${id}`, {
      method: 'DELETE',
      headers: {
        'Content-Type': 'application/json'
      }
    })

    return responseHandler(response)
  }
}

export const serverFetcher = async <T = void>(action: () => Promise<T>) => {
  try {
    return await action()
  } catch (error: any) {
    // Always log errors for debugging
    console.error('Server Fetcher Error:', error)
    console.error('Error details:', {
      name: error?.constructor?.name,
      message: error?.message || 'Unknown error',
      stack: error?.stack?.split('\n').slice(0, 5).join('\n') || 'No stack trace'
    })
    
    if (error instanceof MidazApiException && error.code === '0042') {
      redirect('/signout')
    }
    return null
  }
}

const responseHandler = async (response: Response) => {
  if (!response.ok) {
    if (response.status === 401) {
      signOut({ callbackUrl: '/login' })
      return
    }

    try {
      const errorMessage = await response.json()
      const message = errorMessage.message || errorMessage.error || 'An error occurred'
      throw new Error(typeof message === 'string' ? message : JSON.stringify(message))
    } catch (e) {
      // If JSON parsing fails or message extraction fails
      throw new Error(`Request failed with status ${response.status}`)
    }
  }

  return await response.json()
}
