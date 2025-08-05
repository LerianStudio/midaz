import { getNextRequestArgument } from '../utils/get-next-arguments'

export type RequestMetadata = {
  propertyKey: string | symbol
  parameterIndex: number
}

const requestKey = Symbol('request')

export function requestDecoratorHandler(
  target: Object,
  propertyKey: string | symbol,
  args: any[]
) {
  const metadata: RequestMetadata = Reflect.getOwnMetadata(
    requestKey,
    target,
    propertyKey
  )
  if (metadata) {
    return {
      parameter: getNextRequestArgument(args),
      parameterIndex: metadata.parameterIndex
    }
  }
  return null
}

export function Request() {
  return function (
    target: Object,
    propertyKey: string | symbol,
    parameterIndex: number
  ) {
    Reflect.defineMetadata(
      requestKey,
      {
        propertyKey,
        parameterIndex
      },
      target,
      propertyKey
    )
  }
}

export { Request as Req }
