import { WorkflowUseCase } from '@/core/application/use-cases/workflows/workflow-use-case'
import { Container, ContainerModule } from '../../utils/di/container'

// Create symbols for injection
export const WORKFLOW_SYMBOLS = {
  WorkflowUseCase: Symbol.for('WorkflowUseCase')
}

export const WorkflowModule = new ContainerModule((container: Container) => {
  container
    .bind<WorkflowUseCase>(WORKFLOW_SYMBOLS.WorkflowUseCase)
    .to(WorkflowUseCase)
})
