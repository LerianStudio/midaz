import { getInitialValues } from '@/lib/form'
import { isNil } from 'lodash'
import { useEffect } from 'react'
import { UseFormReturn } from 'react-hook-form'

/**
 * Hook to populate form according to a control variable called mode.
 * Useful in scenarios where the form is used for both create and edit operations.
 * Works akin to [usePopulateForm](../../lib/form/use-populate-form.ts) hook.
 * @param form form from react-hook-form
 * @param mode Mode variable to determine if the form is in create or edit mode
 * @param initialValues Initial values object
 * @param data Data to pre-populate the form when edit mode
 */
export function usePopulateCreateUpdateForm(
  form: UseFormReturn<any>,
  mode: 'create' | 'edit',
  initialValues: any,
  data: any
) {
  useEffect(() => {
    if (mode === 'create') {
      form.reset(initialValues)
    }
  }, [mode])

  useEffect(() => {
    if (mode === 'edit' && !isNil(data)) {
      form.reset(getInitialValues(initialValues, data))
    }
  }, [data, mode])
}
