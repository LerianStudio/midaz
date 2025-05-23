import { isNil } from 'lodash'
import { useEffect } from 'react'
import { UseFormReturn } from 'react-hook-form'
import { getInitialValues } from './get-initial-values'

/**
 * Pre-populate a form using external data.
 * Useful for editing forms.
 * @param form The form object
 * @param data Data to populate the form
 */
export function usePopulateForm(form: UseFormReturn<any>, data: any) {
  useEffect(() => {
    if (isNil(data)) {
      return
    }

    form.reset(getInitialValues(form.formState.defaultValues, data), {
      keepDefaultValues: true
    })
  }, [data])
}
