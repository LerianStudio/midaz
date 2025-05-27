import { HolderEntity } from '@/core/domain/entities/holder-entity'
import { AliasEntity } from '@/core/domain/entities/alias-entity'

export function exportHoldersToCSV(holders: HolderEntity[]): string {
  const headers = [
    'ID',
    'Type',
    'Name',
    'Document',
    'Trading Name',
    'Legal Name',
    'Website',
    'Address Line 1',
    'Address Line 2',
    'City',
    'State',
    'Zip Code',
    'Country',
    'Status',
    'Created At'
  ]

  const rows = holders.map(holder => [
    holder.id,
    holder.type,
    holder.name,
    holder.document,
    holder.tradingName || '',
    holder.legalName || '',
    holder.website || '',
    holder.address?.line1 || '',
    holder.address?.line2 || '',
    holder.address?.city || '',
    holder.address?.state || '',
    holder.address?.zipCode || '',
    holder.address?.country || '',
    holder.status || 'ACTIVE',
    holder.createdAt ? new Date(holder.createdAt).toISOString() : ''
  ])

  const csvContent = [
    headers.join(','),
    ...rows.map(row => row.map(cell => `"${cell}"`).join(','))
  ].join('\n')

  return csvContent
}

export function exportAliasesToCSV(aliases: AliasEntity[], holders: Record<string, HolderEntity>): string {
  const headers = [
    'Alias ID',
    'Alias Document',
    'Type',
    'Holder Name',
    'Holder Document',
    'Ledger ID',
    'Account ID',
    'Bank ID',
    'Bank Branch',
    'Account Number',
    'Account Type',
    'Created At'
  ]

  const rows = aliases.map(alias => {
    const holder = holders[alias.holderId || '']
    return [
      alias.id,
      alias.document,
      alias.type,
      holder?.name || 'Unknown',
      holder?.document || '',
      alias.ledgerId,
      alias.accountId,
      alias.bankingDetails?.bankId || '',
      alias.bankingDetails?.branch || '',
      alias.bankingDetails?.account || '',
      alias.bankingDetails?.type || '',
      alias.createdAt ? new Date(alias.createdAt).toISOString() : ''
    ]
  })

  const csvContent = [
    headers.join(','),
    ...rows.map(row => row.map(cell => `"${cell}"`).join(','))
  ].join('\n')

  return csvContent
}

export function downloadCSV(content: string, filename: string) {
  const blob = new Blob([content], { type: 'text/csv;charset=utf-8;' })
  const link = document.createElement('a')
  const url = URL.createObjectURL(blob)
  
  link.setAttribute('href', url)
  link.setAttribute('download', filename)
  link.style.visibility = 'hidden'
  
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
}