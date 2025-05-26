import React from 'react'

export type UseTimeProps = {
  interval?: number
  onUpdate?: (time: Date) => void
}

export const useTime = ({ interval = 1000, onUpdate }: UseTimeProps) => {
  const [time, setTime] = React.useState<Date>(new Date())

  React.useEffect(() => {
    const timer = setInterval(() => {
      const newTime = new Date()
      setTime(newTime)
      onUpdate?.(newTime)
    }, interval)

    return () => clearInterval(timer)
  }, [interval, onUpdate])

  return time
}
