/**
 * TODO: Better error handling
 */
import { MidazApiException } from '@/core/infrastructure/midaz/exceptions/midaz-exceptions'
import { signOut } from 'next-auth/react'
import { redirect } from 'next/navigation'
import { createQueryString } from '../search'
import { AuthApiException } from '@/core/infrastructure/midaz-plugins/auth/exceptions/auth-exceptions'

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
  } catch (error) {
    // Only log errors when not in test environment
    if (process.env.NODE_ENV !== 'test') {
      console.error('Server Fetcher Error', error)
    }
    if (error instanceof AuthApiException) {
      redirect('/signout')
    }
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

    const errorMessage = await response.json()
    throw new Error(errorMessage.message)
  }

  return await response.json()
}
