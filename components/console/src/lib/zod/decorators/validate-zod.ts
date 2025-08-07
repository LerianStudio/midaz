import { ZodSchema } from 'zod'
import { ValidationApiException } from '@/lib/http'

export function ValidateZod(schema: ZodSchema): MethodDecorator {
  return function (
    target: Object,
    propertyKey: string | symbol | undefined,
    descriptor: PropertyDescriptor
  ) {
    const originalMethod = descriptor.value

    descriptor.value = async function (request: Request, ...args: any[]) {
      const body = await request.clone().json()

      const parsed = schema.safeParse(body)
      if (!parsed.success) {
        // If validation fails, throw an error
        throw new ValidationApiException(
          `Invalid body: ${JSON.stringify(parsed.error.flatten().fieldErrors)}`
        )
      }

      return await originalMethod.apply(this, [request, ...args])
    }
  }
}
