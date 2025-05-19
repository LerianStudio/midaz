import React from 'react'

export const useDebounce = (
  callback: Function,
  milliSeconds: number,
  dependencyArray: any[]
) => {
  return React.useEffect(() => {
    const handler = setTimeout(async () => {
      await callback()
    }, milliSeconds)

    return () => {
      clearTimeout(handler)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [milliSeconds, ...dependencyArray])
}
