/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: false,
  logging: {
    fetches: {
      fullUrl: true
    }
  },
  env: {
    MIDAZ_CONSOLE_BASE_PATH: process.env.MIDAZ_CONSOLE_BASE_PATH,
    MIDAZ_SERVER_BASE_PATH: process.env.MIDAZ_SERVER_BASE_PATH,
    PLUGIN_AUTH_ENABLED: process.env.PLUGIN_AUTH_ENABLED,
    VERSION: process.env.VERSION
  },
  headers: async () => {
    return [
      {
        source: '/api/:path*',
        headers: [
          { key: 'Access-Control-Allow-Credentials', value: 'true' },
          { key: 'Access-Control-Allow-Origin', value: '*' }, // replace this your actual origin
          {
            key: 'Access-Control-Allow-Methods',
            value: 'GET,DELETE,PATCH,POST,PUT'
          },
          {
            key: 'Access-Control-Allow-Headers',
            value:
              'X-CSRF-Token, X-Requested-With, Accept, Accept-Version, Content-Length, Content-MD5, Content-Type, Date, X-Api-Version'
          }
        ]
      }
    ]
  },
  images: {
    dangerouslyAllowSVG: true,
    contentSecurityPolicy: "default-src 'self'; script-src 'none'; sandbox;",
    contentDispositionType: 'attachment',

    remotePatterns: [
      {
        protocol: 'http',
        hostname: '**',
        pathname: '**'
      },
      {
        protocol: 'https',
        hostname: '**',
        pathname: '**'
      }
    ]
  },
  compiler: {
    reactRemoveProperties:
      process.env.NODE_ENV === 'production'
        ? { properties: ['^data-testid$'] }
        : false
  },
  webpack: (config, { isServer }) => {
    config.resolve.fallback = {
      ...config.resolve.fallback,
      worker_threads: false,
      pino: false
    }

    return config
  },

  experimental: {
    instrumentationHook: true,
    serverComponentsExternalPackages: [
      'pino',
      'pino-pretty',
      '@opentelemetry/instrumentation',
      '@opentelemetry/api',
      '@opentelemetry/api-logs',
      '@opentelemetry/exporter-logs-otlp-http',
      '@opentelemetry/exporter-metrics-otlp-http',
      '@opentelemetry/exporter-trace-otlp-http',
      '@opentelemetry/instrumentation',
      '@opentelemetry/instrumentation-http',
      '@opentelemetry/instrumentation-pino',
      '@opentelemetry/instrumentation-runtime-node',
      '@opentelemetry/resources',
      '@opentelemetry/sdk-logs',
      '@opentelemetry/sdk-metrics',
      '@opentelemetry/sdk-node',
      '@opentelemetry/sdk-trace-base',
      '@opentelemetry/instrumentation-undici'
    ]
  }
}

export default nextConfig
