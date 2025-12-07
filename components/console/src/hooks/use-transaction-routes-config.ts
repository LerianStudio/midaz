import { useMidazConfig } from '@/hooks/use-midaz-config'
import { useTransactionRoutesCursor } from '@/hooks/use-transaction-routes-cursor'
import { useOrganization } from '@lerianstudio/console-layout'

export function useTransactionRoutesConfig() {
  const { currentOrganization, currentLedger } = useOrganization()

  // 1. Usar o hook existente para config e reutilizar isAccountTypeValidationEnabled
  const {
    config,
    isAccountTypeValidationEnabled,
    isLoading: isLoadingConfig
  } = useMidazConfig()

  // 2. Usar isAccountTypeValidationEnabled diretamente (já tem a lógica completa)
  const shouldUseRoutes = isAccountTypeValidationEnabled

  // 3. Usar o hook existente para listar transaction routes
  const {
    transactionRoutes,
    isLoading: isLoadingRoutes,
    error: routesError
  } = useTransactionRoutesCursor({
    organizationId: currentOrganization?.id || '',
    ledgerId: currentLedger?.id || '',
    enabled: shouldUseRoutes, // Só busca se shouldUseRoutes === true
    limit: 100, // Limite de 100 conforme requisito
    sortOrder: 'asc'
  })

  return {
    shouldUseRoutes,
    transactionRoutes,
    isLoading: isLoadingConfig || (shouldUseRoutes && isLoadingRoutes),
    error: routesError,
    config
  }
}
