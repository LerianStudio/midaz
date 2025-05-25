import {
  WorkflowTemplate,
  TemplateCategory,
  TemplateComplexity
} from '../entities/workflow-template'

export const mockWorkflowTemplates: WorkflowTemplate[] = [
  {
    id: 'template-payment-processing',
    name: 'Payment Processing Workflow',
    description:
      'Complete payment processing flow with validation, authorization, and settlement',
    category: 'payments' as TemplateCategory,
    tags: ['payment', 'authorization', 'settlement', 'validation'],
    workflow: {
      name: 'Payment Processing',
      description:
        'Handles end-to-end payment processing with proper validation and error handling',
      tasks: [
        {
          name: 'validate_payment_request',
          type: 'HTTP_TASK',
          description: 'Validate payment request data and format',
          configurable: {
            endpoint: true,
            method: false,
            headers: true,
            body: true,
            timeout: true,
            retries: true
          },
          defaultConfiguration: {
            method: 'POST',
            timeout: 30000,
            retryCount: 3
          }
        },
        {
          name: 'check_account_balance',
          type: 'HTTP_TASK',
          description: 'Verify sufficient account balance for the transaction',
          configurable: {
            endpoint: true,
            headers: true,
            timeout: true
          },
          defaultConfiguration: {
            method: 'GET',
            timeout: 15000
          }
        },
        {
          name: 'authorize_payment',
          type: 'HTTP_TASK',
          description: 'Authorize payment with payment processor',
          configurable: {
            endpoint: true,
            headers: true,
            body: true,
            timeout: true,
            retries: true
          },
          defaultConfiguration: {
            method: 'POST',
            timeout: 45000,
            retryCount: 2
          }
        },
        {
          name: 'settlement_decision',
          type: 'SWITCH',
          description:
            'Route to appropriate settlement method based on payment type',
          configurable: {
            body: true
          },
          defaultConfiguration: {
            evaluatorType: 'value-param',
            expression: 'payment_type'
          }
        },
        {
          name: 'process_settlement',
          type: 'HTTP_TASK',
          description: 'Process payment settlement',
          configurable: {
            endpoint: true,
            method: false,
            headers: true,
            body: true,
            timeout: true
          },
          defaultConfiguration: {
            method: 'POST',
            timeout: 60000
          }
        },
        {
          name: 'update_transaction_status',
          type: 'HTTP_TASK',
          description: 'Update transaction status in ledger',
          configurable: {
            endpoint: true,
            headers: true,
            body: true
          },
          defaultConfiguration: {
            method: 'PUT',
            timeout: 15000
          }
        },
        {
          name: 'send_confirmation',
          type: 'HTTP_TASK',
          description: 'Send payment confirmation to customer',
          optional: true,
          configurable: {
            endpoint: true,
            headers: true,
            body: true
          },
          defaultConfiguration: {
            method: 'POST',
            timeout: 10000
          }
        }
      ],
      inputParameters: [
        'payment_amount',
        'payment_type',
        'account_id',
        'customer_id'
      ],
      outputParameters: ['transaction_id', 'status', 'authorization_code'],
      timeoutSeconds: 300
    },
    parameters: [
      {
        name: 'payment_amount',
        type: 'number',
        required: true,
        description: 'Amount to be processed',
        validation: {
          min: 0.01,
          max: 100000
        }
      },
      {
        name: 'payment_type',
        type: 'select',
        required: true,
        description: 'Type of payment processing',
        options: [
          {
            label: 'Credit Card',
            value: 'credit_card',
            description: 'Process via credit card'
          },
          {
            label: 'Bank Transfer',
            value: 'bank_transfer',
            description: 'Process via bank transfer'
          },
          {
            label: 'Digital Wallet',
            value: 'digital_wallet',
            description: 'Process via digital wallet'
          },
          {
            label: 'Cryptocurrency',
            value: 'crypto',
            description: 'Process via cryptocurrency'
          }
        ]
      },
      {
        name: 'account_id',
        type: 'string',
        required: true,
        description: 'Source account identifier',
        validation: {
          pattern: '^ACC[0-9]{8}$',
          minLength: 11,
          maxLength: 11
        }
      },
      {
        name: 'customer_id',
        type: 'string',
        required: true,
        description: 'Customer identifier',
        validation: {
          pattern: '^CUST[0-9]{8}$'
        }
      },
      {
        name: 'notification_enabled',
        type: 'boolean',
        required: false,
        description: 'Enable customer notifications',
        defaultValue: true
      }
    ],
    usageCount: 2847,
    rating: 4.8,
    createdBy: 'system',
    createdAt: '2024-01-15T10:00:00Z',
    updatedAt: '2024-01-20T14:30:00Z',
    isPublic: true,
    metadata: {
      version: '2.1.0',
      schemaVersion: '1.0',
      complexity: 'MEDIUM' as TemplateComplexity,
      estimatedDuration: '2-5 minutes',
      requiredServices: [
        'payment-gateway',
        'ledger-service',
        'notification-service'
      ],
      supportedFormats: ['JSON', 'XML'],
      documentation:
        'Complete payment processing workflow with comprehensive error handling and notifications.',
      examples: [
        {
          name: 'Basic Credit Card Payment',
          description: 'Process a standard credit card payment',
          input: {
            payment_amount: 99.99,
            payment_type: 'credit_card',
            account_id: 'ACC12345678',
            customer_id: 'CUST87654321',
            notification_enabled: true
          },
          expectedOutput: {
            transaction_id: 'TXN98765432',
            status: 'COMPLETED',
            authorization_code: 'AUTH123456'
          }
        }
      ]
    }
  },
  {
    id: 'template-customer-onboarding',
    name: 'Customer Onboarding Workflow',
    description:
      'Complete customer onboarding process with KYC verification and account setup',
    category: 'onboarding' as TemplateCategory,
    tags: ['kyc', 'onboarding', 'verification', 'account-setup'],
    workflow: {
      name: 'Customer Onboarding',
      description:
        'Comprehensive customer onboarding with identity verification and account creation',
      tasks: [
        {
          name: 'collect_customer_info',
          type: 'HUMAN_TASK',
          description: 'Collect basic customer information and documents',
          configurable: {
            timeout: true
          },
          defaultConfiguration: {
            assignmentTimeoutSeconds: 86400
          }
        },
        {
          name: 'verify_identity',
          type: 'HTTP_TASK',
          description: 'Verify customer identity through third-party service',
          configurable: {
            endpoint: true,
            headers: true,
            body: true,
            timeout: true,
            retries: true
          },
          defaultConfiguration: {
            method: 'POST',
            timeout: 60000,
            retryCount: 2
          }
        },
        {
          name: 'kyc_decision',
          type: 'DECISION',
          description: 'Evaluate KYC verification results',
          configurable: {
            body: true
          },
          defaultConfiguration: {
            evaluatorType: 'javascript',
            expression: '$.kyc_score >= 0.8 && $.identity_verified === true'
          }
        },
        {
          name: 'create_customer_profile',
          type: 'HTTP_TASK',
          description: 'Create customer profile in CRM system',
          configurable: {
            endpoint: true,
            headers: true,
            body: true
          },
          defaultConfiguration: {
            method: 'POST',
            timeout: 30000
          }
        },
        {
          name: 'setup_accounts',
          type: 'SUB_WORKFLOW',
          description: 'Set up customer accounts and ledgers',
          configurable: {
            body: true
          },
          defaultConfiguration: {
            subWorkflowParam: {
              name: 'account-setup-workflow'
            }
          }
        },
        {
          name: 'generate_credentials',
          type: 'HTTP_TASK',
          description: 'Generate login credentials and API keys',
          configurable: {
            endpoint: true,
            headers: true,
            body: true
          },
          defaultConfiguration: {
            method: 'POST',
            timeout: 15000
          }
        },
        {
          name: 'send_welcome_notification',
          type: 'HTTP_TASK',
          description: 'Send welcome email and setup instructions',
          optional: true,
          configurable: {
            endpoint: true,
            headers: true,
            body: true
          },
          defaultConfiguration: {
            method: 'POST',
            timeout: 10000
          }
        }
      ],
      inputParameters: ['customer_data', 'verification_level', 'account_types'],
      outputParameters: ['customer_id', 'account_ids', 'credentials'],
      timeoutSeconds: 172800
    },
    parameters: [
      {
        name: 'customer_data',
        type: 'object',
        required: true,
        description: 'Customer personal and business information'
      },
      {
        name: 'verification_level',
        type: 'select',
        required: true,
        description: 'Level of verification required',
        options: [
          {
            label: 'Basic',
            value: 'basic',
            description: 'Basic identity verification'
          },
          {
            label: 'Enhanced',
            value: 'enhanced',
            description: 'Enhanced due diligence'
          },
          {
            label: 'Premium',
            value: 'premium',
            description: 'Premium verification with additional checks'
          }
        ],
        defaultValue: 'basic'
      },
      {
        name: 'account_types',
        type: 'multiselect',
        required: true,
        description: 'Types of accounts to create',
        options: [
          { label: 'Checking Account', value: 'checking' },
          { label: 'Savings Account', value: 'savings' },
          { label: 'Business Account', value: 'business' },
          { label: 'Investment Account', value: 'investment' }
        ]
      },
      {
        name: 'welcome_package',
        type: 'boolean',
        required: false,
        description: 'Send welcome package with documentation',
        defaultValue: true
      }
    ],
    usageCount: 1523,
    rating: 4.6,
    createdBy: 'system',
    createdAt: '2024-01-10T09:00:00Z',
    updatedAt: '2024-01-18T11:15:00Z',
    isPublic: true,
    metadata: {
      version: '1.8.0',
      schemaVersion: '1.0',
      complexity: 'COMPLEX' as TemplateComplexity,
      estimatedDuration: '1-2 days',
      requiredServices: [
        'kyc-service',
        'crm-service',
        'account-service',
        'notification-service'
      ],
      supportedFormats: ['JSON'],
      documentation:
        'Comprehensive customer onboarding workflow with flexible verification levels.'
    }
  },
  {
    id: 'template-compliance-reporting',
    name: 'Compliance Reporting Workflow',
    description:
      'Automated generation and submission of regulatory compliance reports',
    category: 'compliance' as TemplateCategory,
    tags: ['compliance', 'reporting', 'regulatory', 'automation'],
    workflow: {
      name: 'Compliance Reporting',
      description:
        'Generate regulatory compliance reports and submit to authorities',
      tasks: [
        {
          name: 'fetch_transaction_data',
          type: 'HTTP_TASK',
          description: 'Fetch transaction data for reporting period',
          configurable: {
            endpoint: true,
            headers: true,
            body: true,
            timeout: true
          },
          defaultConfiguration: {
            method: 'GET',
            timeout: 120000
          }
        },
        {
          name: 'validate_data_completeness',
          type: 'HTTP_TASK',
          description: 'Validate data completeness and accuracy',
          configurable: {
            endpoint: true,
            body: true
          },
          defaultConfiguration: {
            method: 'POST',
            timeout: 60000
          }
        },
        {
          name: 'generate_report',
          type: 'HTTP_TASK',
          description: 'Generate compliance report in required format',
          configurable: {
            endpoint: true,
            body: true,
            timeout: true
          },
          defaultConfiguration: {
            method: 'POST',
            timeout: 300000
          }
        },
        {
          name: 'review_required',
          type: 'DECISION',
          description: 'Determine if manual review is required',
          configurable: {
            body: true
          },
          defaultConfiguration: {
            evaluatorType: 'javascript',
            expression:
              '$.anomalies_detected === true || $.high_risk_transactions > 0'
          }
        },
        {
          name: 'manual_review',
          type: 'HUMAN_TASK',
          description: 'Manual review of flagged items',
          optional: true,
          configurable: {
            timeout: true
          },
          defaultConfiguration: {
            assignmentTimeoutSeconds: 172800
          }
        },
        {
          name: 'submit_report',
          type: 'HTTP_TASK',
          description: 'Submit report to regulatory authority',
          configurable: {
            endpoint: true,
            headers: true,
            body: true,
            timeout: true,
            retries: true
          },
          defaultConfiguration: {
            method: 'POST',
            timeout: 60000,
            retryCount: 3
          }
        },
        {
          name: 'archive_report',
          type: 'HTTP_TASK',
          description: 'Archive report for record keeping',
          configurable: {
            endpoint: true,
            body: true
          },
          defaultConfiguration: {
            method: 'POST',
            timeout: 30000
          }
        }
      ],
      inputParameters: [
        'reporting_period',
        'report_type',
        'regulatory_authority'
      ],
      outputParameters: [
        'report_id',
        'submission_status',
        'confirmation_number'
      ],
      timeoutSeconds: 86400
    },
    parameters: [
      {
        name: 'reporting_period',
        type: 'object',
        required: true,
        description: 'Start and end dates for reporting period'
      },
      {
        name: 'report_type',
        type: 'select',
        required: true,
        description: 'Type of compliance report',
        options: [
          {
            label: 'AML Report',
            value: 'aml',
            description: 'Anti-Money Laundering report'
          },
          {
            label: 'SAR',
            value: 'sar',
            description: 'Suspicious Activity Report'
          },
          {
            label: 'CTR',
            value: 'ctr',
            description: 'Currency Transaction Report'
          },
          {
            label: 'FBAR',
            value: 'fbar',
            description: 'Foreign Bank Account Report'
          }
        ]
      },
      {
        name: 'regulatory_authority',
        type: 'select',
        required: true,
        description: 'Target regulatory authority',
        options: [
          { label: 'FinCEN', value: 'fincen' },
          { label: 'SEC', value: 'sec' },
          { label: 'CFTC', value: 'cftc' },
          { label: 'OCC', value: 'occ' }
        ]
      },
      {
        name: 'auto_submit',
        type: 'boolean',
        required: false,
        description: 'Automatically submit report without manual review',
        defaultValue: false
      }
    ],
    usageCount: 892,
    rating: 4.9,
    createdBy: 'system',
    createdAt: '2024-01-05T08:00:00Z',
    updatedAt: '2024-01-25T16:45:00Z',
    isPublic: true,
    metadata: {
      version: '3.2.0',
      schemaVersion: '1.0',
      complexity: 'COMPLEX' as TemplateComplexity,
      estimatedDuration: '4-24 hours',
      requiredServices: [
        'transaction-service',
        'compliance-service',
        'document-service'
      ],
      supportedFormats: ['JSON', 'XML', 'PDF'],
      documentation:
        'Automated compliance reporting with manual review capabilities for high-risk scenarios.'
    }
  },
  {
    id: 'template-reconciliation',
    name: 'Daily Reconciliation Workflow',
    description:
      'Daily transaction reconciliation between internal systems and external partners',
    category: 'reconciliation' as TemplateCategory,
    tags: ['reconciliation', 'daily', 'automation', 'validation'],
    workflow: {
      name: 'Daily Reconciliation',
      description:
        'Automated daily reconciliation process with exception handling',
      tasks: [
        {
          name: 'fetch_internal_transactions',
          type: 'HTTP_TASK',
          description: 'Fetch internal transaction records for the day',
          configurable: {
            endpoint: true,
            headers: true,
            body: true
          },
          defaultConfiguration: {
            method: 'GET',
            timeout: 60000
          }
        },
        {
          name: 'fetch_external_records',
          type: 'HTTP_TASK',
          description: 'Fetch external partner transaction records',
          configurable: {
            endpoint: true,
            headers: true,
            body: true
          },
          defaultConfiguration: {
            method: 'GET',
            timeout: 90000
          }
        },
        {
          name: 'perform_matching',
          type: 'HTTP_TASK',
          description: 'Perform automated transaction matching',
          configurable: {
            endpoint: true,
            body: true,
            timeout: true
          },
          defaultConfiguration: {
            method: 'POST',
            timeout: 300000
          }
        },
        {
          name: 'identify_exceptions',
          type: 'HTTP_TASK',
          description: 'Identify unmatched transactions and discrepancies',
          configurable: {
            endpoint: true,
            body: true
          },
          defaultConfiguration: {
            method: 'POST',
            timeout: 120000
          }
        },
        {
          name: 'exception_review',
          type: 'DECISION',
          description: 'Check if exceptions require manual review',
          configurable: {
            body: true
          },
          defaultConfiguration: {
            evaluatorType: 'javascript',
            expression:
              '$.exceptions_count > 0 && $.total_exception_amount > $.review_threshold'
          }
        },
        {
          name: 'manual_exception_review',
          type: 'HUMAN_TASK',
          description: 'Manual review of significant exceptions',
          optional: true,
          configurable: {
            timeout: true
          },
          defaultConfiguration: {
            assignmentTimeoutSeconds: 28800
          }
        },
        {
          name: 'generate_reconciliation_report',
          type: 'HTTP_TASK',
          description: 'Generate daily reconciliation report',
          configurable: {
            endpoint: true,
            body: true
          },
          defaultConfiguration: {
            method: 'POST',
            timeout: 60000
          }
        },
        {
          name: 'notify_stakeholders',
          type: 'HTTP_TASK',
          description: 'Send reconciliation summary to stakeholders',
          configurable: {
            endpoint: true,
            headers: true,
            body: true
          },
          defaultConfiguration: {
            method: 'POST',
            timeout: 15000
          }
        }
      ],
      inputParameters: ['reconciliation_date', 'partners', 'review_threshold'],
      outputParameters: ['matched_count', 'exception_count', 'report_id'],
      timeoutSeconds: 3600
    },
    parameters: [
      {
        name: 'reconciliation_date',
        type: 'string',
        required: true,
        description: 'Date for reconciliation (YYYY-MM-DD format)',
        validation: {
          pattern: '^\\d{4}-\\d{2}-\\d{2}$'
        }
      },
      {
        name: 'partners',
        type: 'multiselect',
        required: true,
        description: 'External partners to reconcile with',
        options: [
          { label: 'Bank A', value: 'bank_a' },
          { label: 'Payment Processor B', value: 'processor_b' },
          { label: 'Card Network C', value: 'network_c' },
          { label: 'Clearinghouse D', value: 'clearing_d' }
        ]
      },
      {
        name: 'review_threshold',
        type: 'number',
        required: false,
        description: 'Amount threshold for manual review',
        defaultValue: 1000,
        validation: {
          min: 0
        }
      },
      {
        name: 'notify_exceptions_only',
        type: 'boolean',
        required: false,
        description: 'Only notify if exceptions are found',
        defaultValue: false
      }
    ],
    usageCount: 3456,
    rating: 4.7,
    createdBy: 'system',
    createdAt: '2024-01-08T07:00:00Z',
    updatedAt: '2024-01-22T13:20:00Z',
    isPublic: true,
    metadata: {
      version: '2.5.0',
      schemaVersion: '1.0',
      complexity: 'MEDIUM' as TemplateComplexity,
      estimatedDuration: '30-60 minutes',
      requiredServices: [
        'reconciliation-service',
        'transaction-service',
        'notification-service'
      ],
      supportedFormats: ['JSON', 'CSV'],
      documentation:
        'Automated daily reconciliation with configurable thresholds and manual review workflows.'
    }
  },
  {
    id: 'template-fraud-detection',
    name: 'Real-time Fraud Detection',
    description:
      'Real-time fraud detection and prevention workflow for transactions',
    category: 'payments' as TemplateCategory,
    tags: ['fraud', 'detection', 'real-time', 'security'],
    workflow: {
      name: 'Fraud Detection',
      description:
        'Real-time fraud analysis with machine learning and rule-based detection',
      tasks: [
        {
          name: 'extract_transaction_features',
          type: 'HTTP_TASK',
          description: 'Extract features from transaction data',
          configurable: {
            endpoint: true,
            body: true
          },
          defaultConfiguration: {
            method: 'POST',
            timeout: 5000
          }
        },
        {
          name: 'ml_fraud_score',
          type: 'HTTP_TASK',
          description: 'Get ML fraud score from fraud detection model',
          configurable: {
            endpoint: true,
            body: true,
            timeout: true
          },
          defaultConfiguration: {
            method: 'POST',
            timeout: 10000
          }
        },
        {
          name: 'rule_based_checks',
          type: 'HTTP_TASK',
          description: 'Apply rule-based fraud detection checks',
          configurable: {
            endpoint: true,
            body: true
          },
          defaultConfiguration: {
            method: 'POST',
            timeout: 5000
          }
        },
        {
          name: 'risk_assessment',
          type: 'DECISION',
          description: 'Assess overall fraud risk level',
          configurable: {
            body: true
          },
          defaultConfiguration: {
            evaluatorType: 'javascript',
            expression: '$.ml_score > 0.8 || $.rule_violations > 2'
          }
        },
        {
          name: 'high_risk_actions',
          type: 'FORK_JOIN',
          description: 'Execute parallel high-risk mitigation actions',
          configurable: {
            body: true
          },
          defaultConfiguration: {
            forkTasks: ['block_transaction', 'alert_customer', 'log_incident']
          }
        },
        {
          name: 'customer_verification',
          type: 'HUMAN_TASK',
          description: 'Request additional customer verification',
          optional: true,
          configurable: {
            timeout: true
          },
          defaultConfiguration: {
            assignmentTimeoutSeconds: 1800
          }
        },
        {
          name: 'update_fraud_profile',
          type: 'HTTP_TASK',
          description: 'Update customer fraud profile with results',
          configurable: {
            endpoint: true,
            body: true
          },
          defaultConfiguration: {
            method: 'PUT',
            timeout: 10000
          }
        }
      ],
      inputParameters: ['transaction_data', 'customer_id', 'risk_tolerance'],
      outputParameters: ['fraud_score', 'risk_level', 'action_taken'],
      timeoutSeconds: 300
    },
    parameters: [
      {
        name: 'transaction_data',
        type: 'object',
        required: true,
        description: 'Complete transaction information for analysis'
      },
      {
        name: 'customer_id',
        type: 'string',
        required: true,
        description: 'Customer identifier for profile lookup'
      },
      {
        name: 'risk_tolerance',
        type: 'select',
        required: false,
        description: 'Risk tolerance level for this check',
        options: [
          {
            label: 'Low',
            value: 'low',
            description: 'Conservative fraud detection'
          },
          {
            label: 'Medium',
            value: 'medium',
            description: 'Balanced fraud detection'
          },
          {
            label: 'High',
            value: 'high',
            description: 'Aggressive fraud detection'
          }
        ],
        defaultValue: 'medium'
      },
      {
        name: 'auto_block_enabled',
        type: 'boolean',
        required: false,
        description: 'Automatically block high-risk transactions',
        defaultValue: true
      }
    ],
    usageCount: 15678,
    rating: 4.9,
    createdBy: 'system',
    createdAt: '2024-01-12T12:00:00Z',
    updatedAt: '2024-01-26T09:30:00Z',
    isPublic: true,
    metadata: {
      version: '4.1.0',
      schemaVersion: '1.0',
      complexity: 'ADVANCED' as TemplateComplexity,
      estimatedDuration: '1-5 minutes',
      requiredServices: [
        'fraud-detection-service',
        'ml-service',
        'customer-service'
      ],
      supportedFormats: ['JSON'],
      documentation:
        'Real-time fraud detection with machine learning and configurable risk thresholds.'
    }
  }
]

export const getTemplatesByCategory = (
  category: TemplateCategory
): WorkflowTemplate[] => {
  return mockWorkflowTemplates.filter(
    (template) => template.category === category
  )
}

export const getTemplateById = (id: string): WorkflowTemplate | undefined => {
  return mockWorkflowTemplates.find((template) => template.id === id)
}

export const getPopularTemplates = (limit: number = 5): WorkflowTemplate[] => {
  return mockWorkflowTemplates
    .sort((a, b) => b.usageCount - a.usageCount)
    .slice(0, limit)
}

export const getTopRatedTemplates = (limit: number = 5): WorkflowTemplate[] => {
  return mockWorkflowTemplates
    .sort((a, b) => b.rating - a.rating)
    .slice(0, limit)
}
