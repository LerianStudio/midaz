import {
  CreateOnboardingOrganization,
  CreateOnboardingOrganizationUseCase
} from '@/core/application/use-cases/onboarding/create-onboarding-organization-use-case'
import { Container, ContainerModule } from '../../utils/di/container'
import {
  CompleteOnboarding,
  CompleteOnboardingUseCase
} from '@/core/application/use-cases/onboarding/complete-onboarding-use-case'

export const OnboardingUseCaseModule = new ContainerModule(
  (container: Container) => {
    container
      .bind<CreateOnboardingOrganization>(CreateOnboardingOrganizationUseCase)
      .toSelf()
    container.bind<CompleteOnboarding>(CompleteOnboardingUseCase).toSelf()
  }
)
