import { useNormalize } from './use-normalize'

export type CustomError = {
  message: string
  metadata?: Record<string, any>
}

export type CustomFormErrors = Record<string, CustomError>

export function useCustomFormError() {
  const {
    data: errors,
    add,
    set,
    remove,
    clear
  } = useNormalize<CustomError>({} as CustomError)

  return {
    errors,
    add,
    set,
    remove,
    clear
  }
}
