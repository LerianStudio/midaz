'use client'

import React, { ReactNode, createContext, useContext, useState } from 'react'

interface FormData {
  name: string
  metadata: Record<string, string> | null
}

interface FormStateContextType {
  formData: FormData
  updateFormData: (newData: Partial<FormData>) => void
  isDirty: boolean
  setDirty: (dirty: boolean) => void
  resetForm: () => void
}

const FormStateContext = createContext<FormStateContextType | undefined>(
  undefined
)

export const useFormState = () => {
  const context = useContext(FormStateContext)
  if (context === undefined) {
    throw new Error('useFormState must be used within a DetailsProvider')
  }
  return context
}

export const FormDetailsProvider: React.FC<{ children: ReactNode }> = ({
  children
}) => {
  const [formData, setFormData] = useState<FormData>({ name: '', metadata: {} })
  const [isDirty, setIsDirty] = useState(false)

  const setDirty = (dirty: boolean) => setIsDirty(dirty)

  const updateFormData = (newData: Partial<FormData>) => {
    setFormData((prevData) => ({ ...prevData, ...newData }))
    setDirty(true)
  }

  const resetForm = () => {
    setFormData({ name: '', metadata: {} })
    setDirty(false)
  }

  return (
    <FormStateContext.Provider
      value={{ formData, updateFormData, isDirty, setDirty, resetForm }}
    >
      {children}
    </FormStateContext.Provider>
  )
}
