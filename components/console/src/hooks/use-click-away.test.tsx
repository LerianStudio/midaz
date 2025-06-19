import { renderHook } from '@testing-library/react'
import { useClickAway } from './use-click-away'
import React from 'react'

describe('useClickAway', () => {
  let ref: React.RefObject<HTMLElement | null>
  let onClickAway: jest.Mock

  beforeEach(() => {
    ref = { current: document.createElement('div') }
    onClickAway = jest.fn()
    document.body.appendChild(ref.current!)
  })

  afterEach(() => {
    document.body.removeChild(ref.current!)
    jest.clearAllMocks()
  })

  it('should call onClickAway when clicking outside the ref element', () => {
    renderHook(() => useClickAway(ref, onClickAway))

    const outsideElement = document.createElement('div')
    document.body.appendChild(outsideElement)

    outsideElement.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))

    expect(onClickAway).toHaveBeenCalledTimes(1)

    document.body.removeChild(outsideElement)
  })

  it('should not call onClickAway when clicking inside the ref element', () => {
    renderHook(() => useClickAway(ref, onClickAway))

    ref.current!.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))

    expect(onClickAway).not.toHaveBeenCalled()
  })

  it('should clean up event listeners on unmount', () => {
    const { unmount } = renderHook(() => useClickAway(ref, onClickAway))

    unmount()

    const outsideElement = document.createElement('div')
    document.body.appendChild(outsideElement)

    outsideElement.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))

    expect(onClickAway).not.toHaveBeenCalled()

    document.body.removeChild(outsideElement)
  })
})
