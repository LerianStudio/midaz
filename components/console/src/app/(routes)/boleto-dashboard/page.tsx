'use client'

import React, { useState } from 'react'
import { Button } from '@/components/ui/button'
import { InputField } from '@/components/form/input-field'
import { SelectField } from '@/components/form/select-field'
import { SelectItem } from '@/components/ui/select'
import { EmptyResource } from '@/components/empty-resource'
import { EntityDataTable } from '@/components/entity-data-table'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Eye, FileText, Copy, Search } from 'lucide-react'
import { useForm, FormProvider } from 'react-hook-form'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { PageHeader } from '@/components/page-header'
import { getBreadcrumbPaths } from '@/components/breadcrumb/get-breadcrumb-paths'
import { Breadcrumb } from '@/components/breadcrumb'

// Types for our boleto data
type BoletoStatus = 'pending' | 'paid' | 'expired' | 'cancelled'

interface Boleto {
  id: string
  code: string
  value: number
  dueDate: string
  payer: string
  status: BoletoStatus
  createdAt: string
}

// Mock data for the boletos
const mockBoletos: Boleto[] = [
  {
    id: '1',
    code: '34191.79001 01043.510047 91020.150008 9 88660000029999',
    value: 299.99,
    dueDate: '2025-06-15',
    payer: 'João Silva',
    status: 'pending',
    createdAt: '2025-05-15'
  },
  {
    id: '2',
    code: '34191.79001 01043.510047 91020.150008 9 88660000015000',
    value: 150.0,
    dueDate: '2025-06-10',
    payer: 'Maria Oliveira',
    status: 'paid',
    createdAt: '2025-05-10'
  },
  {
    id: '3',
    code: '34191.79001 01043.510047 91020.150008 9 88660000050000',
    value: 500.0,
    dueDate: '2025-05-20',
    payer: 'Carlos Santos',
    status: 'expired',
    createdAt: '2025-04-20'
  },
  {
    id: '4',
    code: '34191.79001 01043.510047 91020.150008 9 88660000010000',
    value: 100.0,
    dueDate: '2025-06-30',
    payer: 'Ana Pereira',
    status: 'pending',
    createdAt: '2025-05-30'
  },
  {
    id: '5',
    code: '34191.79001 01043.510047 91020.150008 9 88660000020000',
    value: 200.0,
    dueDate: '2025-06-05',
    payer: 'Paulo Ferreira',
    status: 'cancelled',
    createdAt: '2025-05-05'
  }
]

// Status badge component
function StatusBadge({ status }: { status: BoletoStatus }) {
  const statusConfig = {
    pending: {
      label: 'Pendente',
      variant: 'secondary' as const,
      className: 'bg-yellow-100 text-yellow-800 hover:bg-yellow-100'
    },
    paid: {
      label: 'Pago',
      variant: 'default' as const,
      className: 'bg-green-100 text-green-800 hover:bg-green-100'
    },
    expired: {
      label: 'Vencido',
      variant: 'destructive' as const,
      className: ''
    },
    cancelled: {
      label: 'Cancelado',
      variant: 'outline' as const,
      className: ''
    }
  }

  const config = statusConfig[status]

  return (
    <Badge variant={config.variant} className={config.className}>
      {config.label}
    </Badge>
  )
}

// Main component
export default function Page() {
  const [loading, setLoading] = useState(false)
  const [boletos, setBoletos] = useState<Boleto[]>(mockBoletos)
  const [filteredBoletos, setFilteredBoletos] = useState<Boleto[]>(mockBoletos)

  const form = useForm({
    defaultValues: {
      code: '',
      status: 'all',
      startDate: '',
      endDate: ''
    }
  })

  const handleFilter = (data: any) => {
    setLoading(true)

    // Simulate API call
    setTimeout(() => {
      let filtered = [...mockBoletos]

      if (data.code) {
        filtered = filtered.filter((boleto) =>
          boleto.code.toLowerCase().includes(data.code.toLowerCase())
        )
      }

      if (data.status && data.status !== 'all') {
        filtered = filtered.filter((boleto) => boleto.status === data.status)
      }

      if (data.startDate) {
        const startDate = new Date(data.startDate)
        filtered = filtered.filter(
          (boleto) => new Date(boleto.dueDate) >= startDate
        )
      }

      if (data.endDate) {
        const endDate = new Date(data.endDate)
        filtered = filtered.filter(
          (boleto) => new Date(boleto.dueDate) <= endDate
        )
      }

      setFilteredBoletos(filtered)
      setLoading(false)
    }, 500)
  }

  const handleReset = () => {
    form.reset()
    setFilteredBoletos(mockBoletos)
  }

  const formatCurrency = (value: number) => {
    return new Intl.NumberFormat('pt-BR', {
      style: 'currency',
      currency: 'BRL'
    }).format(value)
  }

  const formatDate = (dateString: string) => {
    return new Date(dateString).toLocaleDateString('pt-BR')
  }

  const copyToClipboard = (text: string) => {
    navigator.clipboard
      .writeText(text)
      .then(() => {
        // Could show a toast notification here
        console.log('Copied to clipboard')
      })
      .catch((err) => {
        console.error('Failed to copy: ', err)
      })
  }

  const columns = [
    {
      header: 'Código',
      accessorKey: 'code',
      cell: ({ row }: any) => {
        const code = row.original.code
        const shortCode = `${code.substring(0, 10)}...`

        return (
          <div className="flex items-center space-x-2">
            <span className="font-mono text-xs">{shortCode}</span>
            <Button
              variant="ghost"
              size="icon"
              onClick={() => copyToClipboard(code)}
              title="Copiar código"
            >
              <Copy className="h-4 w-4" />
            </Button>
          </div>
        )
      }
    },
    {
      header: 'Valor',
      accessorKey: 'value',
      cell: ({ row }: any) => formatCurrency(row.original.value)
    },
    {
      header: 'Vencimento',
      accessorKey: 'dueDate',
      cell: ({ row }: any) => formatDate(row.original.dueDate)
    },
    {
      header: 'Pagador',
      accessorKey: 'payer'
    },
    {
      header: 'Status',
      accessorKey: 'status',
      cell: ({ row }: any) => <StatusBadge status={row.original.status} />
    },
    {
      header: 'Criado em',
      accessorKey: 'createdAt',
      cell: ({ row }: any) => formatDate(row.original.createdAt)
    },
    {
      header: 'Ações',
      id: 'actions',
      cell: ({ row }: any) => {
        return (
          <div className="flex space-x-2">
            <Button variant="ghost" size="icon" title="Visualizar boleto">
              <Eye className="h-4 w-4" />
            </Button>
            <Button variant="ghost" size="icon" title="Baixar PDF">
              <FileText className="h-4 w-4" />
            </Button>
          </div>
        )
      }
    }
  ]

  return (
    <>
      <PageHeader.Root>
        <Breadcrumb
          paths={[
            { name: 'Home', href: '/' },
            { name: 'Dashboard de Boletos', href: '/boleto-dashboard' }
          ]}
        />
        <PageHeader.Wrapper>
          <PageHeader.InfoTitle
            title="Dashboard de Boletos"
            subtitle="Gerencie e acompanhe todos os boletos emitidos pela sua empresa."
          />
        </PageHeader.Wrapper>
      </PageHeader.Root>

      <div className="container mx-auto p-6">
        <Card className="mb-6">
          <CardHeader>
            <CardTitle>Filtros</CardTitle>
          </CardHeader>
          <CardContent>
            <FormProvider {...form}>
              <form onSubmit={form.handleSubmit(handleFilter)}>
                <div className="grid grid-cols-1 gap-6 md:grid-cols-4">
                  <div>
                    <InputField
                      name="code"
                      control={form.control}
                      label="Código do Boleto"
                      placeholder="Digite o código"
                    />
                  </div>

                  <div>
                    <SelectField
                      name="status"
                      control={form.control}
                      label="Status"
                    >
                      <SelectItem value="all">Todos</SelectItem>
                      <SelectItem value="pending">Pendente</SelectItem>
                      <SelectItem value="paid">Pago</SelectItem>
                      <SelectItem value="expired">Vencido</SelectItem>
                      <SelectItem value="cancelled">Cancelado</SelectItem>
                    </SelectField>
                  </div>

                  <div>
                    <InputField
                      name="startDate"
                      control={form.control}
                      type="date"
                      label="Vencimento (de)"
                    />
                  </div>

                  <div>
                    <InputField
                      name="endDate"
                      control={form.control}
                      type="date"
                      label="Vencimento (até)"
                    />
                  </div>
                </div>

                <div className="mt-6 flex justify-end space-x-4">
                  <Button
                    type="button"
                    variant="outline"
                    onClick={handleReset}
                    disabled={loading}
                  >
                    Limpar Filtros
                  </Button>
                  <Button
                    type="submit"
                    icon={<Search className="h-4 w-4" />}
                    disabled={loading}
                  >
                    Filtrar
                  </Button>
                </div>
              </form>
            </FormProvider>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Boletos</CardTitle>
          </CardHeader>
          <CardContent>
            {loading ? (
              <div className="space-y-4">
                <Skeleton className="h-10 w-full" />
                <Skeleton className="h-10 w-full" />
                <Skeleton className="h-10 w-full" />
                <Skeleton className="h-10 w-full" />
              </div>
            ) : filteredBoletos.length > 0 ? (
              <EntityDataTable.Root>
                <table className="w-full caption-bottom text-sm">
                  <thead className="[&_tr]:border-b">
                    <tr className="border-b transition-colors hover:bg-muted/50 data-[state=selected]:bg-muted">
                      {columns.map((column) => (
                        <th
                          key={column.accessorKey || column.id}
                          className="h-12 px-4 text-left align-middle font-medium text-muted-foreground"
                        >
                          {column.header}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody className="[&_tr:last-child]:border-0">
                    {filteredBoletos.map((row) => (
                      <tr
                        key={row.id}
                        className="border-b transition-colors hover:bg-muted/50 data-[state=selected]:bg-muted"
                      >
                        {columns.map((column) => (
                          <td
                            key={column.accessorKey || column.id}
                            className="p-4 align-middle"
                          >
                            {column.cell
                              ? column.cell({ row: { original: row } })
                              : (row as any)[column.accessorKey as string]}
                          </td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </EntityDataTable.Root>
            ) : (
              <EmptyResource message="Nenhum boleto encontrado">
                <p className="text-sm text-shadcn-500">
                  Tente ajustar os filtros para encontrar o que está procurando.
                </p>
              </EmptyResource>
            )}
          </CardContent>
        </Card>
      </div>
    </>
  )
}
