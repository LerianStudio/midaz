import z from 'zod'
import { getNextRequestArgument } from '../utils/get-next-arguments'
import { ValidationApiException } from '../../api-exception'

export type QueryMetadata = {
  propertyKey: string | symbol
  parameterIndex: number
  schema?: z.ZodSchema
}

const queryKey = Symbol('query')

export function queryDecoratorHandler(
  target: Object,
  propertyKey: string | symbol,
  args: any[]
) {
  const metadata: QueryMetadata = Reflect.getOwnMetadata(
    queryKey,
    target,
    propertyKey
  )
  if (metadata) {
    const request = getNextRequestArgument(args)
    const { searchParams } = new URL(request.url)

    const query = Object.fromEntries(searchParams.entries())

    if (!metadata.schema) {
      return {
        parameter: query,
        parameterIndex: metadata.parameterIndex
      }
    }

    const parsedQuery = metadata.schema?.safeParse(query)

    if (!parsedQuery?.success) {
      throw new ValidationApiException(
        `Invalid query parameters: ${JSON.stringify(
          parsedQuery.error.flatten().fieldErrors
        )}`
      )
    }

    return {
      parameter: parsedQuery.data,
      parameterIndex: metadata.parameterIndex
    }
  }

  return null
}

export function Query(schema?: z.ZodSchema) {
  return function (
    target: Object,
    propertyKey: string | symbol,
    parameterIndex: number
  ) {
    Reflect.defineMetadata(
      queryKey,
      {
        propertyKey,
        parameterIndex,
        schema
      },
      target,
      propertyKey
    )
  }
}
