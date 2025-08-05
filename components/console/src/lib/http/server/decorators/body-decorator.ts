import { z } from 'zod'
import { getNextRequestArgument } from '../utils/get-next-arguments'
import { ValidationApiException } from '../../api-exception'

export type BodyMetadata = {
  propertyIndex: number
  schema?: z.ZodSchema
}

const bodyKey = Symbol('body')

export async function bodyDecoratorHandler(
  target: Object,
  propertyKey: string | symbol,
  args: any[]
) {
  const metadata: BodyMetadata = Reflect.getOwnMetadata(
    bodyKey,
    target,
    propertyKey
  )

  if (metadata) {
    const request = getNextRequestArgument(args)
    const body = await request.json()

    if (!metadata.schema) {
      return {
        parameter: body,
        parameterIndex: metadata.propertyIndex
      }
    }

    const parsedBody = metadata.schema?.safeParse(body)

    if (!parsedBody?.success) {
      throw new ValidationApiException(
        `Invalid body: ${JSON.stringify(parsedBody.error.flatten().fieldErrors)}`
      )
    }

    return {
      parameter: parsedBody.data,
      parameterIndex: metadata.propertyIndex
    }
  }

  return null
}

export function Body(schema?: z.ZodSchema) {
  return function (
    target: Object,
    propertyKey: string | symbol,
    propertyIndex: number
  ) {
    Reflect.defineMetadata(
      bodyKey,
      { propertyIndex, schema },
      target,
      propertyKey
    )
  }
}
