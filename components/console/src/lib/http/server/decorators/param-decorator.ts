import { ValidationApiException } from '../../api-exception'
import { getNextParamArgument } from '../utils/get-next-arguments'

export type ParamMetadata = {
  name: string
  parameterIndex: number
}

const paramKey = Symbol('param')

export async function paramDecoratorHandler(
  target: Object,
  propertyKey: string | symbol,
  args: any[]
): Promise<any> {
  const metadatas: ParamMetadata[] | undefined = Reflect.getOwnMetadata(
    paramKey,
    target,
    propertyKey
  )

  if (metadatas && metadatas.length > 0) {
    const params: { [key: string]: any } = await getNextParamArgument(args)

    return metadatas.map((metadata) => {
      const value = params[metadata.name]

      if (!value) {
        throw new ValidationApiException(
          `Invalid param: ${metadata.name} is required`
        )
      }

      return {
        parameter: value,
        parameterIndex: metadata.parameterIndex
      }
    })
  }

  return null
}

export function Param(name: string) {
  return function (
    target: Object,
    propertyKey: string | symbol,
    parameterIndex: number
  ) {
    const existingParams: ParamMetadata[] =
      Reflect.getOwnMetadata(paramKey, target, propertyKey) || []

    existingParams.push({
      name,
      parameterIndex
    })

    Reflect.defineMetadata(paramKey, existingParams, target, propertyKey)
  }
}
