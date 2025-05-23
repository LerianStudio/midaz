import { OtelTracerProvider, SpanData } from './otel-tracer-provider'
import { trace, Span, SpanStatusCode } from '@opentelemetry/api'

jest.mock('@opentelemetry/api', () => ({
  trace: {
    getTracer: jest.fn()
  },
  SpanStatusCode: {
    OK: 'OK',
    UNSET: 'UNSET'
  }
}))

describe('OtelTracerProvider', () => {
  let originalEnv: NodeJS.ProcessEnv
  let mockSpan: jest.Mocked<Span>

  beforeAll(() => {
    originalEnv = { ...process.env }
  })

  afterAll(() => {
    process.env = originalEnv
  })

  beforeEach(() => {
    jest.resetModules()

    mockSpan = {
      setAttributes: jest.fn(),
      setStatus: jest.fn(),
      end: jest.fn()
    } as unknown as jest.Mocked<Span>
    ;(trace.getTracer as jest.Mock).mockReturnValue({
      startSpan: jest.fn().mockReturnValue(mockSpan)
    })
  })

  afterEach(() => {
    jest.clearAllMocks()
  })

  it('should initialize tracer when telemetry is enabled', () => {
    process.env.ENABLE_TELEMETRY = 'true'

    const provider = new OtelTracerProvider()

    expect(provider).toBeDefined()
    expect(trace.getTracer).toHaveBeenCalledWith('midaz-console')
  })

  it('should not initialize tracer when telemetry is disabled', () => {
    process.env.ENABLE_TELEMETRY = 'false'

    const provider = new OtelTracerProvider()

    expect(trace.getTracer).not.toHaveBeenCalled()
    expect(provider).toBeDefined()
  })

  it('should start a custom span when telemetry is enabled', () => {
    process.env.ENABLE_TELEMETRY = 'true'

    const provider = new OtelTracerProvider()
    provider.startCustomSpan('test-span')

    expect(trace.getTracer('midaz-console').startSpan).toHaveBeenCalledWith(
      'test-span'
    )

    const testSpanData: SpanData = {
      attributes: { foo: 'bar' },
      status: { code: SpanStatusCode.OK }
    }

    provider.endCustomSpan(testSpanData)

    expect(mockSpan.setAttributes).toHaveBeenCalledWith(testSpanData.attributes)
    expect(mockSpan.setStatus).toHaveBeenCalledWith(testSpanData.status)
    expect(mockSpan.end).toHaveBeenCalled()
  })

  it('should not start a custom span when telemetry is disabled', () => {
    process.env.ENABLE_TELEMETRY = 'false'

    const provider = new OtelTracerProvider()

    provider.startCustomSpan('span-disables')

    expect(trace.getTracer('midaz-console').startSpan).not.toHaveBeenCalled()
  })
})
