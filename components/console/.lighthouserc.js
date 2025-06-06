module.exports = {
  ci: {
    collect: {
      startServerCommand: 'npm run start', // Command to start the server
      url: ['http://localhost:8081'], // URL to execute the Lighthouse audit
      numberOfRuns: 3
    },
    assert: {
      assertions: {
        'bf-cache': ['warn', { minScore: 0 }],
        'color-contrast': ['warn', { minScore: 0 }],
        'efficient-animated-content': ['warn', { maxLength: 1 }],
        'errors-in-console': ['warn', { minScore: 0 }],
        'html-has-lang': ['warn', { minScore: 0 }],
        'meta-description': ['warn', { minScore: 0 }],
        'robots-txt': ['warn', { minScore: 0 }],
        'unused-javascript': ['warn', { maxLength: 1 }],
        'uses-rel-preconnect': ['warn', { maxLength: 1 }],
        'uses-responsive-images': ['warn', { maxLength: 1 }],
        interactive: ['warn', { minScore: 0 }],
        'largest-contentful-paint': ['warn', { minScore: 0 }],
        'legacy-javascript': ['warn', { maxLength: 1 }],
        'render-blocking-resources': ['warn', { maxLength: 1 }]
      }
    },
    upload: [
      {
        target: 'temporary-public-storage' // Upload para o Google Cloud Storage
      }
    ]
  }
}
