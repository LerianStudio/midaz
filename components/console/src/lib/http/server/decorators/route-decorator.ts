import { NextResponse } from 'next/server'
import { bodyDecoratorHandler } from './body-decorator'
import { paramDecoratorHandler } from './param-decorator'
import { queryDecoratorHandler } from './query-decorator'
import { requestDecoratorHandler } from './request-decorator'

export function Route(routeKey: symbol): MethodDecorator {
  return function (
    target: Object,
    propertyKey: string | symbol,
    descriptor: PropertyDescriptor
  ) {
    Reflect.defineMetadata(
      routeKey,
      {
        method: routeKey
      },
      target,
      propertyKey
    )

    const originalMethod = descriptor.value

    descriptor.value = async function (...originalArgs: any[]) {
      const args = [
        await requestDecoratorHandler(target, propertyKey, originalArgs),
        await queryDecoratorHandler(target, propertyKey, originalArgs),
        await paramDecoratorHandler(target, propertyKey, originalArgs),
        await bodyDecoratorHandler(target, propertyKey, originalArgs)
      ]
        .flat()
        .filter((a) => a !== null && a !== undefined)
        .sort((a, b) => a.parameterIndex - b.parameterIndex)
        .map((a) => a.parameter)

      const response = await originalMethod.apply(this, args)

      if (response instanceof NextResponse) {
        return response
      }

      return NextResponse.json(response)
    }
  }
}

export function Get() {
  return Route(Symbol('GET'))
}

export function Post() {
  return Route(Symbol('POST'))
}

export function Put() {
  return Route(Symbol('PUT'))
}

export function Patch() {
  return Route(Symbol('PATCH'))
}

export function Delete() {
  return Route(Symbol('DELETE'))
}
