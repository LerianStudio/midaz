import {
  WorkflowTemplate,
  TemplateCategory,
  TemplateComplexity
} from '../entities/workflow-template'

export const crossServiceTemplates: WorkflowTemplate[] = [
  {
    id: 'template-customer-onboarding-complete',
    name: 'Complete Customer Onboarding',
    description:
      'End-to-end customer onboarding with KYC verification, account creation, and initial product setup across multiple services',
    category: 'onboarding' as TemplateCategory,
    tags: ['kyc', 'onboarding', 'multi-service', 'identity', 'crm', 'account'],
    workflow: {
      name: 'Customer Onboarding Flow',
      description:
        'Orchestrates customer onboarding across Identity, CRM, Auth, and Account services',
      tasks: [
        {
          name: 'verify_customer_identity',
          type: 'HTTP_TASK',
          description: 'Verify customer identity with Identity service',
          configurable: {
            endpoint: true,
            headers: true,
            body: true,
            timeout: true
          },
          defaultConfiguration: {
            method: 'POST',
            endpoint: 'http://plugin-identity:4001/v1/verify',
            timeout: 30000,
            headers: {
              'Content-Type': 'application/json'
            }
          }
        },
        {
          name: 'kyc_decision',
          type: 'SWITCH',
          description: 'Route based on KYC verification result',
          configurable: {},
          defaultConfiguration: {
            evaluatorType: 'value-param',
            expression: 'verify_customer_identity.output.response.body.status',
            decisionCases: {
              approved: ['create_crm_record'],
              rejected: ['send_rejection_notice'],
              pending: ['manual_review_task']
            }
          }
        },
        {
          name: 'create_crm_record',
          type: 'HTTP_TASK',
          description: 'Create customer record in CRM service',
          configurable: {
            endpoint: true,
            headers: true,
            body: true,
            timeout: true
          },
          defaultConfiguration: {
            method: 'POST',
            endpoint: 'http://plugin-crm:4003/v1/customers',
            timeout: 15000,
            headers: {
              'Content-Type': 'application/json'
            }
          }
        },
        {
          name: 'create_auth_user',
          type: 'HTTP_TASK',
          description: 'Create authentication user with Auth service',
          configurable: {
            endpoint: true,
            headers: true,
            body: true,
            timeout: true
          },
          defaultConfiguration: {
            method: 'POST',
            endpoint: 'http://plugin-auth:4002/v1/users',
            timeout: 15000,
            headers: {
              'Content-Type': 'application/json'
            }
          }
        },
        {
          name: 'create_organization',
          type: 'HTTP_TASK',
          description: 'Create organization in core Onboarding service',
          configurable: {
            endpoint: true,
            headers: true,
            body: true,
            timeout: true
          },
          defaultConfiguration: {
            method: 'POST',
            endpoint: 'http://midaz-onboarding:3000/v1/organizations',
            timeout: 15000
          }
        },
        {
          name: 'create_ledger',
          type: 'HTTP_TASK',
          description: 'Create financial ledger for the organization',
          configurable: {
            endpoint: true,
            headers: true,
            body: true,
            timeout: true
          },
          defaultConfiguration: {
            method: 'POST',
            endpoint:
              'http://midaz-onboarding:3000/v1/organizations/${create_organization.output.response.body.id}/ledgers',
            timeout: 15000
          }
        },
        {
          name: 'create_primary_account',
          type: 'HTTP_TASK',
          description: 'Create primary account for the customer',
          configurable: {
            endpoint: true,
            headers: true,
            body: true,
            timeout: true
          },
          defaultConfiguration: {
            method: 'POST',
            endpoint:
              'http://midaz-onboarding:3000/v1/organizations/${create_organization.output.response.body.id}/ledgers/${create_ledger.output.response.body.id}/accounts',
            timeout: 15000
          }
        },
        {
          name: 'send_welcome_email',
          type: 'HTTP_TASK',
          description: 'Send welcome email with account details',
          configurable: {
            endpoint: true,
            headers: true,
            body: true,
            timeout: true
          },
          defaultConfiguration: {
            method: 'POST',
            endpoint: 'http://notification-service/v1/emails/send',
            timeout: 10000
          }
        },
        {
          name: 'send_rejection_notice',
          type: 'HTTP_TASK',
          description: 'Send rejection notification',
          configurable: {
            endpoint: true,
            headers: true,
            body: true,
            timeout: true
          },
          defaultConfiguration: {
            method: 'POST',
            endpoint: 'http://notification-service/v1/emails/send',
            timeout: 10000
          }
        },
        {
          name: 'manual_review_task',
          type: 'HUMAN',
          description: 'Manual review required for pending KYC',
          configurable: {
            timeout: true
          },
          defaultConfiguration: {
            timeout: 86400000 // 24 hours
          }
        }
      ]
    },
    parameters: [
      {
        name: 'customerData',
        type: 'object',
        required: true,
        description:
          'Customer information including personal details and documents',
        validation: {
          pattern: '.*'
        }
      },
      {
        name: 'productType',
        type: 'select',
        required: true,
        description: 'Type of account/product to set up',
        options: [
          { value: 'checking', label: 'Checking Account' },
          { value: 'savings', label: 'Savings Account' },
          { value: 'business', label: 'Business Account' }
        ]
      },
      {
        name: 'initialDeposit',
        type: 'number',
        required: false,
        description: 'Initial deposit amount',
        defaultValue: 0,
        validation: {
          min: 0,
          max: 1000000
        }
      }
    ],
    metadata: {
      version: '1.0.0',
      schemaVersion: '1.0',
      complexity: 'COMPLEX' as TemplateComplexity,
      estimatedDuration: '5-10 minutes',
      requiredServices: [
        'identity-service',
        'crm-service',
        'auth-service',
        'onboarding-service'
      ],
      supportedFormats: ['json']
    },
    usageCount: 234,
    rating: 4.8,
    createdBy: 'admin@midaz.com',
    createdAt: new Date('2024-10-01').toISOString(),
    updatedAt: new Date('2024-12-28').toISOString(),
    isPublic: true
  },
  {
    id: 'template-payment-with-fees',
    name: 'Payment Processing with Dynamic Fees',
    description:
      'Process payments with dynamic fee calculation, fraud detection, and multi-currency support',
    category: 'payments' as TemplateCategory,
    tags: [
      'payment',
      'fees',
      'fraud-detection',
      'multi-currency',
      'transaction'
    ],
    workflow: {
      name: 'Payment with Fees Flow',
      description:
        'Orchestrates payment processing with fee calculation and fraud detection',
      tasks: [
        {
          name: 'validate_accounts',
          type: 'HTTP_TASK',
          description: 'Validate source and destination accounts',
          configurable: {
            endpoint: true,
            headers: true,
            timeout: true
          },
          defaultConfiguration: {
            method: 'GET',
            endpoint:
              'http://midaz-onboarding:3000/v1/organizations/${organizationId}/accounts/${sourceAccountId}',
            timeout: 15000
          }
        },
        {
          name: 'check_fraud_score',
          type: 'HTTP_TASK',
          description: 'Check fraud score with external service',
          configurable: {
            endpoint: true,
            headers: true,
            body: true,
            timeout: true
          },
          defaultConfiguration: {
            method: 'POST',
            endpoint: 'http://fraud-detection-service/v1/score',
            timeout: 5000
          }
        },
        {
          name: 'fraud_decision',
          type: 'SWITCH',
          description: 'Route based on fraud score',
          configurable: {},
          defaultConfiguration: {
            evaluatorType: 'javascript',
            expression: '$.check_fraud_score.output.response.body.score < 70',
            decisionCases: {
              true: ['calculate_fees'],
              false: ['manual_fraud_review']
            }
          }
        },
        {
          name: 'calculate_fees',
          type: 'HTTP_TASK',
          description: 'Calculate transaction fees',
          configurable: {
            endpoint: true,
            headers: true,
            body: true,
            timeout: true
          },
          defaultConfiguration: {
            method: 'POST',
            endpoint: 'http://plugin-fees:4004/v1/fees/calculate',
            timeout: 10000
          }
        },
        {
          name: 'currency_conversion',
          type: 'HTTP_TASK',
          description: 'Convert currency if needed',
          configurable: {
            endpoint: true,
            headers: true,
            body: true,
            timeout: true
          },
          defaultConfiguration: {
            method: 'POST',
            endpoint: 'http://currency-service/v1/convert',
            timeout: 5000
          }
        },
        {
          name: 'create_transaction',
          type: 'HTTP_TASK',
          description: 'Create the actual transaction',
          configurable: {
            endpoint: true,
            headers: true,
            body: true,
            timeout: true
          },
          defaultConfiguration: {
            method: 'POST',
            endpoint:
              'http://midaz-transaction:3001/v1/organizations/${organizationId}/ledgers/${ledgerId}/transactions',
            timeout: 30000
          }
        },
        {
          name: 'update_balances',
          type: 'SUB_WORKFLOW',
          description: 'Update account balances',
          configurable: {},
          defaultConfiguration: {
            subWorkflowName: 'balance_update_workflow',
            subWorkflowVersion: 1
          }
        },
        {
          name: 'send_notifications',
          type: 'FORK_JOIN',
          description: 'Send notifications to all parties',
          configurable: {},
          defaultConfiguration: {
            forkTasks: [
              ['notify_sender', 'notify_receiver', 'notify_compliance']
            ]
          }
        },
        {
          name: 'notify_sender',
          type: 'HTTP_TASK',
          description: 'Notify sender',
          configurable: {
            endpoint: true,
            body: true
          },
          defaultConfiguration: {
            method: 'POST',
            endpoint: 'http://notification-service/v1/notify'
          }
        },
        {
          name: 'notify_receiver',
          type: 'HTTP_TASK',
          description: 'Notify receiver',
          configurable: {
            endpoint: true,
            body: true
          },
          defaultConfiguration: {
            method: 'POST',
            endpoint: 'http://notification-service/v1/notify'
          }
        },
        {
          name: 'notify_compliance',
          type: 'HTTP_TASK',
          description: 'Notify compliance if needed',
          configurable: {
            endpoint: true,
            body: true
          },
          defaultConfiguration: {
            method: 'POST',
            endpoint: 'http://compliance-service/v1/report'
          }
        },
        {
          name: 'join',
          type: 'JOIN',
          description: 'Wait for all notifications',
          configurable: {},
          defaultConfiguration: {}
        },
        {
          name: 'manual_fraud_review',
          type: 'HUMAN',
          description: 'Manual fraud review required',
          configurable: {
            timeout: true
          },
          defaultConfiguration: {
            timeout: 3600000 // 1 hour
          }
        }
      ]
    },
    parameters: [
      {
        name: 'sourceAccountId',
        type: 'string',
        required: true,
        description: 'Source account identifier'
      },
      {
        name: 'destinationAccountId',
        type: 'string',
        required: true,
        description: 'Destination account identifier'
      },
      {
        name: 'amount',
        type: 'number',
        required: true,
        description: 'Transaction amount',
        validation: {
          min: 0.01,
          max: 1000000
        }
      },
      {
        name: 'currency',
        type: 'string',
        required: true,
        description: 'Transaction currency (ISO 4217)',
        defaultValue: 'USD'
      },
      {
        name: 'description',
        type: 'string',
        required: false,
        description: 'Transaction description',
        validation: {
          maxLength: 500
        }
      },
      {
        name: 'metadata',
        type: 'object',
        required: false,
        description: 'Additional transaction metadata'
      }
    ],
    metadata: {
      version: '2.0.0',
      schemaVersion: '1.0',
      complexity: 'ADVANCED' as TemplateComplexity,
      estimatedDuration: '2-5 minutes',
      requiredServices: [
        'transaction-service',
        'fees-service',
        'fraud-service',
        'notification-service'
      ],
      supportedFormats: ['json']
    },
    usageCount: 567,
    rating: 4.9,
    createdBy: 'admin@midaz.com',
    createdAt: new Date('2024-09-15').toISOString(),
    updatedAt: new Date('2024-12-30').toISOString(),
    isPublic: true
  },
  {
    id: 'template-reconciliation-workflow',
    name: 'Multi-Source Reconciliation',
    description:
      'Reconcile transactions from multiple sources with automatic matching and exception handling',
    category: 'reconciliation' as TemplateCategory,
    tags: ['reconciliation', 'matching', 'automation', 'reporting'],
    workflow: {
      name: 'Reconciliation Workflow',
      description:
        'Automated reconciliation with multiple data sources and intelligent matching',
      tasks: [
        {
          name: 'fetch_internal_transactions',
          type: 'HTTP_TASK',
          description: 'Fetch transactions from internal system',
          configurable: {
            endpoint: true,
            headers: true,
            body: true,
            timeout: true
          },
          defaultConfiguration: {
            method: 'GET',
            endpoint:
              'http://midaz-transaction:3001/v1/organizations/${organizationId}/transactions',
            timeout: 30000
          }
        },
        {
          name: 'fetch_external_transactions',
          type: 'FORK_JOIN_DYNAMIC',
          description: 'Fetch from multiple external sources',
          configurable: {},
          defaultConfiguration: {
            dynamicForkTasksParam: 'externalSources',
            dynamicForkTasksInputParamName: 'sourceConfig'
          }
        },
        {
          name: 'fetch_bank_transactions',
          type: 'HTTP_TASK',
          description: 'Fetch from bank API',
          configurable: {
            endpoint: true,
            headers: true,
            timeout: true
          },
          defaultConfiguration: {
            method: 'GET',
            endpoint: '${sourceConfig.endpoint}',
            timeout: 45000
          }
        },
        {
          name: 'transform_external_data',
          type: 'LAMBDA',
          description: 'Transform external data to standard format',
          configurable: {},
          defaultConfiguration: {
            scriptParam: 'transformScript'
          }
        },
        {
          name: 'join_external_sources',
          type: 'JOIN',
          description: 'Wait for all external sources',
          configurable: {},
          defaultConfiguration: {}
        },
        {
          name: 'run_matching_engine',
          type: 'HTTP_TASK',
          description: 'Run reconciliation matching engine',
          configurable: {
            endpoint: true,
            headers: true,
            body: true,
            timeout: true
          },
          defaultConfiguration: {
            method: 'POST',
            endpoint:
              'http://plugin-reconciliation:4005/v1/reconciliation/match',
            timeout: 60000
          }
        },
        {
          name: 'analyze_exceptions',
          type: 'HTTP_TASK',
          description: 'Analyze unmatched transactions',
          configurable: {
            endpoint: true,
            headers: true,
            body: true,
            timeout: true
          },
          defaultConfiguration: {
            method: 'POST',
            endpoint:
              'http://plugin-reconciliation:4005/v1/reconciliation/analyze-exceptions',
            timeout: 30000
          }
        },
        {
          name: 'exception_routing',
          type: 'SWITCH',
          description: 'Route exceptions based on type',
          configurable: {},
          defaultConfiguration: {
            evaluatorType: 'javascript',
            expression:
              '$.analyze_exceptions.output.response.body.exceptionType',
            decisionCases: {
              amount_mismatch: ['create_adjustment'],
              missing_transaction: ['investigate_missing'],
              duplicate: ['mark_duplicate'],
              timing_difference: ['schedule_retry']
            },
            defaultCase: ['manual_review']
          }
        },
        {
          name: 'create_adjustment',
          type: 'HTTP_TASK',
          description: 'Create adjustment entry',
          configurable: {
            endpoint: true,
            body: true
          },
          defaultConfiguration: {
            method: 'POST',
            endpoint: 'http://plugin-reconciliation:4005/v1/adjustments'
          }
        },
        {
          name: 'generate_report',
          type: 'HTTP_TASK',
          description: 'Generate reconciliation report',
          configurable: {
            endpoint: true,
            headers: true,
            body: true,
            timeout: true
          },
          defaultConfiguration: {
            method: 'POST',
            endpoint: 'http://plugin-reconciliation:4005/v1/reports/generate',
            timeout: 45000
          }
        },
        {
          name: 'store_report',
          type: 'HTTP_TASK',
          description: 'Store report in document service',
          configurable: {
            endpoint: true,
            headers: true,
            body: true
          },
          defaultConfiguration: {
            method: 'POST',
            endpoint: 'http://document-service/v1/documents'
          }
        },
        {
          name: 'send_report',
          type: 'HTTP_TASK',
          description: 'Send report to stakeholders',
          configurable: {
            endpoint: true,
            body: true
          },
          defaultConfiguration: {
            method: 'POST',
            endpoint:
              'http://notification-service/v1/emails/send-with-attachment'
          }
        }
      ]
    },
    parameters: [
      {
        name: 'reconciliationDate',
        type: 'string',
        required: true,
        description: 'Date to reconcile (YYYY-MM-DD)'
      },
      {
        name: 'externalSources',
        type: 'multiselect',
        required: true,
        description: 'External sources to reconcile',
        options: [
          { value: 'bank_a', label: 'Bank A' },
          { value: 'bank_b', label: 'Bank B' },
          { value: 'payment_processor', label: 'Payment Processor' },
          { value: 'card_network', label: 'Card Network' }
        ]
      },
      {
        name: 'matchingThreshold',
        type: 'number',
        required: false,
        description: 'Matching confidence threshold (0-100)',
        defaultValue: 95,
        validation: {
          min: 50,
          max: 100
        }
      },
      {
        name: 'reportRecipients',
        type: 'string',
        required: true,
        description: 'Email addresses for report (comma-separated)'
      }
    ],
    metadata: {
      version: '1.5.0',
      schemaVersion: '1.0',
      complexity: 'ADVANCED' as TemplateComplexity,
      estimatedDuration: '15-30 minutes',
      requiredServices: [
        'reconciliation-service',
        'transaction-service',
        'document-service'
      ],
      supportedFormats: ['json']
    },
    usageCount: 89,
    rating: 4.7,
    createdBy: 'admin@midaz.com',
    createdAt: new Date('2024-11-01').toISOString(),
    updatedAt: new Date('2024-12-29').toISOString(),
    isPublic: true
  }
]
