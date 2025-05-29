import React from 'react'

type UseConfirmDialog = {
  onConfirm?: (id: string) => void
}

/**
 * A custom hook that manages the state and behavior of a confirmation dialog.
 *
 * @template TData - The type of the data associated with the dialog.
 *
 * @param {UseConfirmDialog} param0 - An object containing the onConfirm callback.
 * @param {Function} [param0.onConfirm] - A callback function that is called when the dialog is confirmed.
 *
 * @returns {Object} An object containing the dialog state and handlers.
 * @returns {string} id - The current id associated with the dialog.
 * @returns {TData | null} data - The current data associated with the dialog.
 * @returns {Function} handleDialogOpen - A function to open the dialog with a specific id and optional data.
 * @returns {Object} dialogProps - An object containing the dialog properties and handlers.
 */
export function useConfirmDialog<TData = {}>({
  onConfirm: onConfirmProp
}: UseConfirmDialog) {
  const [id, setId] = React.useState('')
  const [data, setData] = React.useState<TData | null>(null)
  const [open, setOpen] = React.useState(false)

  const onOpenChange = (open: boolean) => setOpen(open)

  const handleDialogOpen = (id: string, data?: any) => {
    setId(id)
    setData(data)
    setOpen(true)
  }

  const handleDialogClose = () => setOpen(false)

  const onCancel = () => {
    setId('')
    setData(null)
    setOpen(false)
  }

  const onConfirm = () => {
    setId('')
    setData(null)
    onConfirmProp?.(id)
  }

  return {
    id,
    data,
    handleDialogClose,
    handleDialogOpen,
    dialogProps: {
      open,
      onOpenChange,
      onCancel,
      onConfirm
    }
  }
}
