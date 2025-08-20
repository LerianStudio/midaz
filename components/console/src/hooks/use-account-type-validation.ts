import { useOrganization } from '@lerianstudio/console-layout'

export const useAccountTypeValidation = () => {
  const { currentOrganization, currentLedger } = useOrganization()

  const isValidationEnabled = () => {

    const isAccountTypeValidationEnabled = 
      process.env.NEXT_PUBLIC_ACCOUNT_TYPE_VALIDATION_ENABLED === 'true'

    if (!isAccountTypeValidationEnabled) {
      return false
    }

    const accountTypeValidation = process.env.NEXT_PUBLIC_MIDAZ_ACCOUNT_TYPE_VALIDATION
    
    if (!accountTypeValidation) {
      return false
    }

    const validationPairs = accountTypeValidation.split(',').map(pair => pair.trim())
    
    const currentPair = `${currentOrganization.id}:${currentLedger.id}`

    const isEnabled = validationPairs.includes(currentPair)
    
    return isEnabled
  }

  return {
    isValidationEnabled: isValidationEnabled(),
    organizationId: currentOrganization?.id,
    ledgerId: currentLedger?.id
  }
}
