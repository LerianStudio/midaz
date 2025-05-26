import React from 'react'

export function normalize<T>(value: T[], key: string): Record<string, T> {
  return value.reduce((sum, item) => {
    const itemKey = item[key as keyof T] as string
    return {
      ...sum,
      [itemKey]: item
    }
  }, {})
}

export function useNormalize<T>(value?: T) {
  const [data, setData] = React.useState<Record<string, T>>(value ?? {})

  const add = (key: string, item: T) => {
    setData((prev) => ({
      ...prev,
      [key]: item
    }))
  }

  const remove = (key: string) => {
    setData((prev) => {
      const newData = { ...prev }
      delete newData[key]
      return newData
    })
  }

  const clear = () => {
    setData({})
  }

  return {
    data,
    set: setData,
    add,
    remove,
    clear
  }
}
