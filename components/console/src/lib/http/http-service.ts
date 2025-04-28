import { createQueryString } from '@/lib/search'
import { HttpStatus } from './http-status'
import {
  ApiException,
  InternalServerErrorApiException,
  NotFoundApiException,
  ServiceUnavailableApiException,
  UnauthorizedApiException,
  UnprocessableEntityApiException
} from './api-exception'

export interface FetchModuleOptions extends RequestInit {
  baseUrl?: URL | string
  search?: object
}

/**
 * HTTP service class to allow easy implementation of custom API repositories
 *
 * Code based from nestjs-fetch:
 * https://github.com/mikehall314/nestjs-fetch/blob/main/lib/fetch.service.ts
 */
export abstract class HttpService {
  protected async request<T>(request: Request): Promise<T> {
    try {
      this.onBeforeFetch(request)

      const response = await fetch(request)

      this.onAfterFetch(request, response)

      // Parse text/plain error responses
      if (response?.headers?.get('content-type')?.includes('text/plain')) {
        const message = await response.text()

        await this.catch(request, response, { message })

        if (response.status === HttpStatus.UNAUTHORIZED) {
          throw new UnauthorizedApiException(message)
        } else if (response.status === HttpStatus.NOT_FOUND) {
          throw new NotFoundApiException(message)
        } else if (response.status === HttpStatus.UNPROCESSABLE_ENTITY) {
          throw new UnprocessableEntityApiException(message)
        } else if (response.status === HttpStatus.INTERNAL_SERVER_ERROR) {
          throw new InternalServerErrorApiException(message)
        }

        throw new ServiceUnavailableApiException(message)
      }

      // Parse application/json error responses
      // NodeJS native fetch does not throw for logic errors
      if (!response.ok) {
        const error = await response.json()

        await this.catch(request, response, error)

        if (response.status === HttpStatus.UNAUTHORIZED) {
          throw new UnauthorizedApiException(error)
        } else if (response.status === HttpStatus.NOT_FOUND) {
          throw new NotFoundApiException(error)
        } else if (response.status === HttpStatus.UNPROCESSABLE_ENTITY) {
          throw new UnprocessableEntityApiException(error)
        } else if (response.status === HttpStatus.INTERNAL_SERVER_ERROR) {
          throw new InternalServerErrorApiException(error)
        }

        throw new ServiceUnavailableApiException(error)
      }

      return await response.json()
    } catch (error: any) {
      if (error instanceof ApiException) {
        throw error
      }

      throw new ServiceUnavailableApiException(error)
    }
  }

  private async createRequest(
    url: URL | string,
    options: FetchModuleOptions
  ): Promise<Request> {
    const { baseUrl, search, ...init } = {
      ...(await this.createDefaults()),
      ...options
    }

    return new Request(new URL(url + createQueryString(search), baseUrl), {
      ...init,
      headers: {
        ...options?.headers,
        ...init?.headers
      }
    })
  }

  protected async createDefaults() {
    return {}
  }

  /**
   * Event triggered before the request is sent
   * @param request The request to be sent
   */
  protected onBeforeFetch(request: Request) {}

  /**
   * Event triggered after the request is sent, but before response JSON parsing
   * @param request The request that was sent
   * @param response The raw response received from the server
   */
  protected onAfterFetch(request: Request, response: Response) {}

  /**
   * Catch function to handle errors from the native fetch API
   * @param request The request that was sent
   * @param response The raw response received from the server
   * @param error Parsed error response from the server
   */
  protected async catch(request: Request, response: Response, error: any) {
    console.error('Request error', { response, error })
  }

  async get<T>(
    url: URL | string,
    options: FetchModuleOptions = {}
  ): Promise<T> {
    const request = await this.createRequest(url, { ...options, method: 'GET' })
    return this.request<T>(request)
  }

  async head(
    url: URL | string,
    options: FetchModuleOptions = {}
  ): Promise<Response> {
    const request = await this.createRequest(url, {
      ...options,
      method: 'HEAD'
    })
    return this.request(request)
  }

  async delete(
    url: URL | string,
    options: FetchModuleOptions = {}
  ): Promise<Response> {
    const request = await this.createRequest(url, {
      ...options,
      method: 'DELETE'
    })
    return this.request(request)
  }

  async patch<T>(
    url: URL | string,
    options: FetchModuleOptions = {}
  ): Promise<T> {
    const request = await this.createRequest(url, {
      ...options,
      method: 'PATCH'
    })
    return this.request<T>(request)
  }

  async put<T>(
    url: URL | string,
    options: FetchModuleOptions = {}
  ): Promise<T> {
    const request = await this.createRequest(url, { ...options, method: 'PUT' })
    return this.request<T>(request)
  }

  async post<T>(
    url: URL | string,
    options: FetchModuleOptions = {}
  ): Promise<T> {
    const request = await this.createRequest(url, {
      ...options,
      method: 'POST'
    })
    return this.request<T>(request)
  }
}
