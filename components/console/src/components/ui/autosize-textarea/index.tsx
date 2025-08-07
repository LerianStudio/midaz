'use client'

import * as React from 'react'
import { cn } from '@/lib/utils'
import { useImperativeHandle } from 'react'

interface UseAutosizeTextAreaProps {
  textAreaRef: React.MutableRefObject<HTMLTextAreaElement | null>
  minHeight?: number
  maxHeight?: number
  triggerAutoSize: string
}

export const useAutosizeTextArea = ({
  textAreaRef,
  triggerAutoSize,
  maxHeight = Number.MAX_SAFE_INTEGER,
  minHeight = 0
}: UseAutosizeTextAreaProps) => {
  const [init, setInit] = React.useState(true)
  React.useEffect(() => {
    const textAreaElement = textAreaRef.current
    if (textAreaElement) {
      if (init) {
        textAreaElement.style.minHeight = `${minHeight}px`
        if (maxHeight > minHeight) {
          textAreaElement.style.maxHeight = `${maxHeight}px`
        }
        setInit(false)
      }
      textAreaElement.style.height = `${minHeight}px`
      const scrollHeight = textAreaElement.scrollHeight

      // Trying to set this with state or a ref will segment an incorrect value.
      if (scrollHeight > maxHeight) {
        textAreaElement.style.height = `${maxHeight}px`
        textAreaElement.style.overflowY = 'auto'
      } else {
        textAreaElement.style.height = `${scrollHeight}px`
        textAreaElement.style.overflowY = 'hidden'
      }
    }
  }, [textAreaRef.current, triggerAutoSize])
}

export type AutosizeTextAreaRef = {
  textArea: HTMLTextAreaElement
  maxHeight: number
  minHeight: number
}

export type AutosizeTextAreaProps = {
  maxHeight?: number
  minHeight?: number
} & React.TextareaHTMLAttributes<HTMLTextAreaElement>

export const AutosizeTextarea = React.forwardRef<
  AutosizeTextAreaRef,
  AutosizeTextAreaProps
>(
  (
    {
      maxHeight = Number.MAX_SAFE_INTEGER,
      minHeight = 36,
      className,
      onChange,
      value,
      ...props
    }: AutosizeTextAreaProps,
    ref: React.Ref<AutosizeTextAreaRef>
  ) => {
    const textAreaRef = React.useRef<HTMLTextAreaElement | null>(null)
    const [triggerAutoSize, setTriggerAutoSize] = React.useState('')

    useAutosizeTextArea({
      textAreaRef,
      triggerAutoSize: triggerAutoSize,
      maxHeight,
      minHeight
    })

    useImperativeHandle(ref, () => ({
      textArea: textAreaRef.current as HTMLTextAreaElement,
      focus: () => textAreaRef?.current?.focus(),
      maxHeight,
      minHeight
    }))

    React.useEffect(() => {
      setTriggerAutoSize(value as string)
    }, [props?.defaultValue, value])

    return (
      <textarea
        {...props}
        value={value}
        ref={textAreaRef}
        className={cn(
          'border-input bg-background ring-offset-background placeholder:text-muted-foreground focus-visible:ring-ring flex h-9 w-full overflow-y-hidden rounded-md border px-3 py-2 text-sm focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-hidden',
          'read-only:cursor-default read-only:bg-zinc-100 read-only:caret-transparent read-only:opacity-50 read-only:select-text read-only:focus:ring-0 read-only:focus:ring-offset-0 read-only:focus:outline-hidden',
          'disabled:cursor-not-allowed disabled:opacity-50',
          className
        )}
        onChange={(e) => {
          setTriggerAutoSize(e.target.value)
          onChange?.(e)
        }}
      />
    )
  }
)
AutosizeTextarea.displayName = 'AutosizeTextarea'
