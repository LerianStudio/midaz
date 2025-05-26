import { OtelTracerProvider } from '../../observability/otel-tracer-provider'
import { Container, ContainerModule } from '../../utils/di/container'

export const OtelModule = new ContainerModule((container: Container) => {
  container.bind<OtelTracerProvider>(OtelTracerProvider).toSelf()
})
