import 'reflect-metadata'
import { applyDecorators } from './apply-decorators'

describe('applyDecorators', () => {
  it('applies multiple class decorators in order', () => {
    const calls: string[] = []
    const decoratorA: ClassDecorator = (target) => {
      Reflect.defineMetadata('A', true, target)
      calls.push('A')
    }
    const decoratorB: ClassDecorator = (target) => {
      Reflect.defineMetadata('B', true, target)
      calls.push('B')
    }
    @applyDecorators(decoratorA, decoratorB)
    class _Test {}
    expect(Reflect.getMetadata('A', _Test)).toBe(true)
    expect(Reflect.getMetadata('B', _Test)).toBe(true)
    expect(calls).toEqual(['A', 'B'])
  })

  it('applies multiple method decorators in order', () => {
    const calls: string[] = []
    const decoratorA: MethodDecorator = (target, propertyKey) => {
      Reflect.defineMetadata('A', true, target, propertyKey)
      calls.push('A')
    }
    const decoratorB: MethodDecorator = (target, propertyKey) => {
      Reflect.defineMetadata('B', true, target, propertyKey)
      calls.push('B')
    }
    class Test {
      @applyDecorators(decoratorA, decoratorB)
      method() {}
    }
    expect(Reflect.getMetadata('A', Test.prototype, 'method')).toBe(true)
    expect(Reflect.getMetadata('B', Test.prototype, 'method')).toBe(true)
    expect(calls).toEqual(['A', 'B'])
  })

  it('applies multiple property decorators in order', () => {
    const calls: string[] = []
    const decoratorA: PropertyDecorator = (target, propertyKey) => {
      Reflect.defineMetadata('A', true, target, propertyKey)
      calls.push('A')
    }
    const decoratorB: PropertyDecorator = (target, propertyKey) => {
      Reflect.defineMetadata('B', true, target, propertyKey)
      calls.push('B')
    }
    class Test {
      @applyDecorators(decoratorA, decoratorB)
      prop!: string
    }
    expect(Reflect.getMetadata('A', Test.prototype, 'prop')).toBe(true)
    expect(Reflect.getMetadata('B', Test.prototype, 'prop')).toBe(true)
    expect(calls).toEqual(['A', 'B'])
  })

  it('does nothing if no decorators are provided', () => {
    expect(() => {
      @applyDecorators()
      class _Test {}
    }).not.toThrow()
  })

  it('throws if a decorator throws when used', () => {
    const throwingDecorator: ClassDecorator = () => {
      throw new Error('Decorator error')
    }
    expect(() => {
      @applyDecorators(throwingDecorator)
      class _Test {}
    }).toThrow('Decorator error')
  })

  it('works with a mix of class, method, and property decorators', () => {
    const calls: string[] = []
    const classDec: ClassDecorator = () => {
      calls.push('class')
    }
    const methodDec: MethodDecorator = () => {
      calls.push('method')
    }
    const propDec: PropertyDecorator = () => calls.push('prop')
    @applyDecorators(classDec)
    class _Test {
      @applyDecorators(methodDec)
      method() {}
      @applyDecorators(propDec)
      prop!: string
    }
    // TypeScript applies decorators in the order: property, then method, then class
    expect(calls).toEqual(['method', 'prop', 'class'])
  })
})
