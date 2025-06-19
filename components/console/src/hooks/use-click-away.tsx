import React from 'react'

/**
 * Detects clicks outside of a specified component and triggers a callback function.
 *
 * @param ref Component reference
 * @param onClickAway Callback function to be called when a click is detected outside the component
 */
export const useClickAway = (
  ref: React.RefObject<HTMLElement | null>,
  onClickAway: (event: MouseEvent | TouchEvent) => void
) => {
  const handleClick = React.useCallback(
    (event: MouseEvent | TouchEvent) => {
      if (ref.current && !ref.current.contains(event.target as Node)) {
        onClickAway(event)
      }
    },
    [ref, onClickAway]
  )

  React.useEffect(() => {
    document.addEventListener('mousedown', handleClick)
    document.addEventListener('touchend', handleClick)

    return () => {
      document.removeEventListener('mousedown', handleClick)
      document.removeEventListener('touchend', handleClick)
    }
  }, [ref, handleClick])
}
