'use client'

import { useEffect, useState } from 'react'
import { debounce, isEmpty, pick } from 'lodash'
import { useForm, UseFormProps } from 'react-hook-form'
import { usePagination } from './use-pagination'
import { useSearchParams } from '@/lib/search/use-search-params'

export type UseQueryParams<SearchParams> = {
  initialValues?: SearchParams | {}
  total: number
  formProps?: Partial<UseFormProps>
  debounce?: number
}

export function useQueryParams<SearchParams = {}>({
  initialValues = {},
  total,
  formProps,
  debounce: debounceProp = 300
}: UseQueryParams<SearchParams>) {
  const pagination = usePagination({ total })
  const { searchParams, updateSearchParams } = useSearchParams()

  /**
   * Internal state to allow form debounce
   * This is the values that should be used when calling useQuery hook for
   * search params.
   */
  const [searchValues, setSearchValues] = useState<
    {
      page: string
      limit: string
    } & SearchParams
  >({
    page: pagination.page.toString(),
    limit: pagination.limit.toString(),
    ...initialValues
  } as any)

  const form = useForm({
    ...formProps,
    defaultValues: {
      ...initialValues,
      page: pagination.page.toString(),
      limit: pagination.limit.toString()
    }
  })

  /**
   * useEffect hook to track pagination changes and update the URL search params
   */
  useEffect(() => {
    const newValues = {
      ...searchValues,
      page: pagination.page.toString(),
      limit: pagination.limit.toString()
    }

    // Avoid updating the URL if the searchParams are empty and the pagination is at the first page
    // Always update after that
    if (!(isEmpty(searchParams) && pagination.page === 1)) {
      updateSearchParams(newValues)
    }

    setSearchValues(newValues)
  }, [pagination.page, pagination.limit])

  /**
   * useEffect hook to track form changes, using the method where the watch function
   * from react-hook-form does not trigger a re-render
   *
   * @see https://react-hook-form.com/docs/useform/watch
   */
  useEffect(() => {
    // This subscription happens after the first render
    // In order to update this code, full page refresh is needed
    // The form changes are debounced to avoid multiple calls from TextFields.
    const { unsubscribe } = form.watch(
      debounce((values) => {
        updateSearchParams(values)
        setSearchValues(values)
      }, debounceProp)
    )

    return () => unsubscribe()
  }, [form.watch, debounceProp])

  /**
   * Responsible to sync the URL with internal state at the first render
   */
  useEffect(() => {
    // Do nothing if no searchParams are found
    if (isEmpty(searchParams)) {
      return
    }

    // Pick only the values that are present in the form:
    // page, limit and anything inside initialValues
    const value = pick(searchParams, [
      'page',
      'limit',
      ...Object.keys(initialValues || [])
    ])

    // Do nothing even if there are params present in the URL
    // but none is related to this form
    if (isEmpty(value)) {
      return
    }

    form.reset({
      ...initialValues,
      page: pagination.page.toString(),
      limit: pagination.limit.toString(),
      ...value
    })
  }, [])

  return { form, searchValues, pagination }
}
