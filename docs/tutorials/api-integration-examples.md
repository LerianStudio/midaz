# API Integration Examples

**Navigation:** [Home](../) > [Tutorials](./README.md) > API Integration Examples

This tutorial provides practical examples of integrating with the Midaz APIs, including authentication, basic CRUD operations, and more complex interactions.

## Table of Contents

- [Introduction](#introduction)
- [Prerequisites](#prerequisites)
- [Authentication](#authentication)
- [Basic API Interactions](#basic-api-interactions)
- [Advanced API Interactions](#advanced-api-interactions)
- [Error Handling](#error-handling)
- [Rate Limiting](#rate-limiting)
- [Idempotency](#idempotency)
- [Webhooks](#webhooks)
- [Best Practices](#best-practices)

## Introduction

The Midaz platform provides RESTful APIs for programmatic interaction with the system. This tutorial demonstrates common API integration patterns and best practices.

## Prerequisites

Before starting this tutorial, you should:

1. Have a valid Midaz account with API access
2. Be familiar with making HTTP requests in your preferred programming language
3. Understand the basics of REST API concepts
4. Have read the [API Reference](../api-reference/README.md) documentation

## Authentication

API requests to Midaz require authentication when the authentication plugin is enabled (`PLUGIN_AUTH_ENABLED=true`). Midaz uses OAuth 2.0 Bearer tokens for authentication.

> **Note:** Detailed authentication documentation, including token acquisition and permissions management, is available in the Auth Plugin repository. The examples below assume you have already obtained a valid access token.

### Including Authentication in Requests

```javascript
// JavaScript example using fetch
const fetchData = async (url) => {
  const response = await fetch(url, {
    headers: {
      'Authorization': 'Bearer YOUR_ACCESS_TOKEN',
      'Content-Type': 'application/json'
    }
  });
  return response.json();
};
```

```python
# Python example using requests
import requests

def fetch_data(url, token):
    headers = {
        'Authorization': f'Bearer {token}',
        'Content-Type': 'application/json'
    }
    response = requests.get(url, headers=headers)
    return response.json()
```

## Basic API Interactions

Let's start with some basic API interactions using the Onboarding API.

### Creating an Organization

```javascript
// JavaScript example
const createOrganization = async () => {
  const url = 'https://api.midaz.io/onboarding/v1/organizations';
  const data = {
    name: 'Acme Corporation',
    description: 'Manufacturing company',
    metadata: {
      industry: 'Manufacturing',
      size: 'Medium'
    }
  };
  
  const response = await fetch(url, {
    method: 'POST',
    headers: {
      'Authorization': 'Bearer YOUR_ACCESS_TOKEN',
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(data)
  });
  
  return response.json();
};
```

### Retrieving an Organization

```python
# Python example
import requests

def get_organization(org_id, token):
    url = f'https://api.midaz.io/onboarding/v1/organizations/{org_id}'
    headers = {
        'Authorization': f'Bearer {token}',
        'Content-Type': 'application/json'
    }
    response = requests.get(url, headers=headers)
    return response.json()
```

### Listing Organizations with Pagination

```javascript
// JavaScript example
const listOrganizations = async (limit = 10, cursor = null) => {
  let url = `https://api.midaz.io/onboarding/v1/organizations?limit=${limit}`;
  
  if (cursor) {
    url += `&cursor=${cursor}`;
  }
  
  const response = await fetch(url, {
    headers: {
      'Authorization': 'Bearer YOUR_ACCESS_TOKEN',
      'Content-Type': 'application/json'
    }
  });
  
  return response.json();
};
```

## Advanced API Interactions

Now let's look at some more complex API interactions using the Transaction API.

### Creating a Transaction

```javascript
// JavaScript example
const createTransaction = async (orgId, ledgerId) => {
  const url = `https://api.midaz.io/v1/organizations/${orgId}/ledgers/${ledgerId}/transactions/json`;
  const data = {
    type: 'transfer',
    sources: [
      {
        accountId: 'source-account-id',
        amount: '100.00',
        assetCode: 'USD'
      }
    ],
    destinations: [
      {
        accountId: 'destination-account-id',
        amount: '100.00',
        assetCode: 'USD'
      }
    ],
    metadata: {
      reference: 'Invoice #12345',
      category: 'Payment'
    }
  };
  
  const response = await fetch(url, {
    method: 'POST',
    headers: {
      'Authorization': 'Bearer YOUR_ACCESS_TOKEN',
      'Content-Type': 'application/json',
      'X-Idempotency-Key': 'unique-request-id-123'
    },
    body: JSON.stringify(data)
  });
  
  return response.json();
};
```

### Creating a Transaction using DSL

```python
# Python example
import requests

def create_transaction_dsl(org_id, ledger_id, token):
    url = f'https://api.midaz.io/v1/organizations/{org_id}/ledgers/{ledger_id}/transactions/dsl'
    
    # DSL transaction content
    dsl_content = '''
    transaction "Simple Transfer" {
      description "Transfer between accounts"
      code "TRANSFER"
      
      send USD 100.00 {
        source {
          from "source-account-id" {
            chart_of_accounts "1000"
            description "Withdrawal from source account"
          }
        }
        
        distribute {
          to "destination-account-id" {
            chart_of_accounts "2000"
            description "Deposit to destination account"
          }
        }
      }
    }
    '''
    
    # Create the multipart/form-data request
    files = {'transaction': ('transaction.dsl', dsl_content, 'text/plain')}
    headers = {
        'Authorization': f'Bearer {token}',
        'X-Idempotency-Key': 'unique-request-id-456'
    }
    
    response = requests.post(url, headers=headers, files=files)
    return response.json()
```

## Error Handling

Proper error handling is crucial for robust API integration.

```javascript
// JavaScript example
const safeApiCall = async (apiFunc) => {
  try {
    const result = await apiFunc();
    return { success: true, data: result };
  } catch (error) {
    console.error('API error:', error);
    
    // Handle different error types
    if (error.response) {
      const status = error.response.status;
      const errorData = await error.response.json();
      
      if (status === 401) {
        // Authentication error - token might be expired
        return { success: false, error: 'Authentication failed', code: 'AUTH_ERROR' };
      } else if (status === 404) {
        // Resource not found
        return { success: false, error: 'Resource not found', code: 'NOT_FOUND' };
      } else if (status === 422) {
        // Validation error
        return { 
          success: false, 
          error: 'Validation failed', 
          code: 'VALIDATION_ERROR',
          details: errorData.fields 
        };
      } else {
        // Other errors
        return { 
          success: false, 
          error: errorData.message || 'Unknown error', 
          code: errorData.code || 'UNKNOWN_ERROR' 
        };
      }
    }
    
    // Network errors
    return { success: false, error: 'Network error', code: 'NETWORK_ERROR' };
  }
};
```

## Rate Limiting

Handle rate limiting gracefully in your API integration.

```python
# Python example
import requests
import time

def rate_limited_request(url, token, max_retries=3):
    headers = {
        'Authorization': f'Bearer {token}',
        'Content-Type': 'application/json'
    }
    
    retries = 0
    while retries < max_retries:
        response = requests.get(url, headers=headers)
        
        if response.status_code == 429:
            # Rate limited, get retry-after header
            retry_after = int(response.headers.get('Retry-After', 5))
            print(f'Rate limited. Retrying after {retry_after} seconds')
            time.sleep(retry_after)
            retries += 1
        else:
            return response.json()
    
    # If we've exhausted all retries
    return {'error': 'Rate limit exceeded after maximum retries'}
```

## Idempotency

Use idempotency keys to prevent duplicate transactions.

```javascript
// JavaScript example
const makeIdempotentRequest = async (url, data, idempotencyKey) => {
  const response = await fetch(url, {
    method: 'POST',
    headers: {
      'Authorization': 'Bearer YOUR_ACCESS_TOKEN',
      'Content-Type': 'application/json',
      'X-Idempotency-Key': idempotencyKey
    },
    body: JSON.stringify(data)
  });
  
  return response.json();
};
```

## Webhooks

If your Midaz instance supports webhooks, you can use them to receive real-time updates.

```javascript
// Node.js Express webhook receiver example
const express = require('express');
const bodyParser = require('body-parser');
const crypto = require('crypto');

const app = express();
app.use(bodyParser.json());

// Verify webhook signature
const verifyWebhookSignature = (request, secret) => {
  const signature = request.headers['x-midaz-signature'];
  const payload = JSON.stringify(request.body);
  
  const hmac = crypto.createHmac('sha256', secret);
  hmac.update(payload);
  const digest = hmac.digest('hex');
  
  return crypto.timingSafeEqual(
    Buffer.from(signature),
    Buffer.from(digest)
  );
};

app.post('/webhooks/midaz', (req, res) => {
  // Replace with your actual webhook secret
  const webhookSecret = 'your_webhook_secret';
  
  if (!verifyWebhookSignature(req, webhookSecret)) {
    return res.status(401).send('Invalid signature');
  }
  
  const event = req.body;
  
  // Handle different event types
  switch (event.type) {
    case 'transaction.created':
      console.log('Transaction created:', event.data.id);
      // Process transaction created event
      break;
    case 'transaction.completed':
      console.log('Transaction completed:', event.data.id);
      // Process transaction completed event
      break;
    default:
      console.log('Unhandled event type:', event.type);
  }
  
  res.sendStatus(200);
});

app.listen(3000, () => {
  console.log('Webhook server running on port 3000');
});
```

## Best Practices

Follow these best practices for successful API integration with Midaz:

1. **Always Use HTTPS**: Secure your API requests using HTTPS
2. **Implement Proper Authentication**: Secure your tokens and implement refresh token logic
3. **Use Idempotency Keys**: For all state-changing operations
4. **Implement Retry Logic**: For transient failures, with exponential backoff
5. **Handle Rate Limiting**: Respect rate limits and implement proper retry mechanisms
6. **Validate Input Data**: Before sending to the API
7. **Implement Proper Error Handling**: For all possible error responses
8. **Monitor API Usage**: Track API calls, errors, and performance
9. **Keep API Versions in Mind**: Be aware of API versioning and deprecation policies
10. **Use Webhooks When Available**: For real-time updates rather than polling

## Next Steps

Now that you've learned the basics of integrating with the Midaz APIs, you can explore more advanced topics:

- [Creating Financial Structures](./creating-financial-structures.md)
- [Implementing Transactions](./implementing-transactions.md)
- [Transaction Processing](../components/transaction/transaction-processing.md)
- [Error Handling Best Practices](../developer-guide/error-handling.md)