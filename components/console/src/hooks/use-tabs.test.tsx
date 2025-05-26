import { screen, render, act } from '@testing-library/react'
import { usePathname, useRouter, useSearchParams } from 'next/navigation'
import { useTabs } from './use-tabs'

jest.mock('next/navigation')

const pushMock = jest.fn()

jest.mocked(usePathname).mockReturnValue('example.com')
jest.mocked(useRouter).mockReturnValue({
  push: pushMock
} as any)

function TestComponent() {
  const { activeTab, handleTabChange } = useTabs({
    initialValue: 'tab1'
  })

  return (
    <>
      <p data-testid="activeTab">{activeTab}</p>
      <button data-testid="changeTab" onClick={() => handleTabChange('tab2')} />
    </>
  )
}

function setup(toString = '') {
  jest.mocked(useSearchParams).mockReturnValue({
    toString: () => toString
  } as any)
  render(<TestComponent />)
  const activeTab = screen.getByTestId('activeTab')
  const button = screen.getByTestId('changeTab')
  return { activeTab, button }
}

describe('useTabs', () => {
  test('should change tabs', async () => {
    const { activeTab, button } = setup()

    expect(useSearchParams).toHaveBeenCalled()
    expect(usePathname).toHaveBeenCalled()
    expect(useRouter).toHaveBeenCalled()

    expect(activeTab.innerHTML).toEqual('tab1')

    await act(() => {
      button.click()
    })

    expect(activeTab.innerHTML).toEqual('tab2')
  })

  test('should change URL', async () => {
    const { button } = setup()

    await act(() => {
      button.click()
    })

    expect(pushMock).toHaveBeenCalledWith(`example.com?tab=tab2`)
  })

  test('should update activeTab from URL', async () => {
    const { activeTab } = setup('tab=tab2')

    expect(activeTab.innerHTML).toEqual('tab2')
  })
})
