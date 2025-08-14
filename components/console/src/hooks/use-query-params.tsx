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
  const { searchParams, updateSearchParams } = useSearchParams()
  
  // Initialize pagination with values from URL if available
  const initialPage = parseInt(searchParams?.page || '1') || 1
  const initialLimit = parseInt(searchParams?.limit || '10') || 10
  
  const pagination = usePagination({ 
    total, 
    initialPage,
    initialLimit 
  })

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
      limit: pagination.limit.toString(),
      ...searchParams
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
    // In order to update this code, full page refresh is needed
    const { unsubscribe } = form.watch(
      debounce((values) => {
        // Sync limit changes with pagination
        if (values.limit && parseInt(values.limit) !== pagination.limit) {
          pagination.setLimit(parseInt(values.limit))
        }
        
        // Sync page changes with pagination
        if (values.page && parseInt(values.page) !== pagination.page) {
          pagination.setPage(parseInt(values.page))
        }
        
        updateSearchParams(values)
        setSearchValues(values)
      }, debounceProp)
    )

    return () => unsubscribe()
  }, [form.watch, debounceProp, pagination.setLimit, pagination.setPage, pagination.limit, pagination.page])

  /**
   * Responsible to sync the URL with internal state at the first render
   */
  useEffect(() => {
    if (isEmpty(searchParams)) {
      return
    }

    // page, limit and anything inside initialValues
    const value = pick(searchParams, [
      'page',
      'limit',
      ...Object.keys(initialValues || [])
    ])

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
