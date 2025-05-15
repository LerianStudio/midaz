/**
 * API client for Midaz integration
 */
import axios from 'axios';
import { v4 as uuidv4 } from 'uuid';
import config from '../config.js';

// Create axios instances for onboarding and transaction services
const createAxiosInstance = (baseURL) => {
  const instance = axios.create({
    baseURL,
    timeout: config.api.timeout,
    headers: config.api.headers
  });

  // Interceptor to add headers to each request
  instance.interceptors.request.use((config) => {
    config.headers['X-Request-Id'] = uuidv4();
    return config;
  });

  // Interceptor for error handling
  instance.interceptors.response.use(
    (response) => response,
    async (error) => {
      const { config: originalConfig, response } = error;
      
      // Enhanced error logging with better formatting
      if (response) {
        console.error('\n--------------------------------');
        console.error(`API Error: ${response.status} ${response.statusText}`);
        console.error(`Endpoint: ${originalConfig.method.toUpperCase()} ${originalConfig.url}`);
        console.error('--------------------------------');
        console.error('Response data:', JSON.stringify(response.data, null, 2));
        console.error('Request data:', JSON.stringify(originalConfig.data, null, 2));
        console.error('--------------------------------\n');
      } else {
        console.error(`\nNetwork error or timeout: ${error.message}\n`);
      }
      
      // Skip retry for specific status codes
      if (response && [400, 401, 403, 404].includes(response.status)) {
        return Promise.reject(error);
      }

      // Implement retry logic with better delay for 500 errors
      if (!originalConfig || !originalConfig._retry) {
        if (!originalConfig._retryCount) {
          originalConfig._retryCount = 0;
        }

        if (originalConfig._retryCount < config.api.retries) {
          originalConfig._retryCount += 1;
          originalConfig._retry = true;

          // More conservative exponential backoff for 500 errors
          let delay = Math.pow(2, originalConfig._retryCount) * 1000;
          
          // Add extra delay for server errors (5xx)
          if (response && response.status >= 500) {
            delay = delay * 2;
          }
          
          console.log(`\n⏱️  Retry attempt ${originalConfig._retryCount}/${config.api.retries} for ${originalConfig.url} after ${delay/1000}s delay`);
          
          return new Promise(resolve => setTimeout(resolve, delay))
            .then(() => instance(originalConfig));
        }
      }

      return Promise.reject(error);
    }
  );

  return instance;
};

// Initialize HTTP clients
const onboardingClient = createAxiosInstance(`${config.api.baseUrl}:${config.api.onboardingPort}`);
const transactionClient = createAxiosInstance(`${config.api.baseUrl}:${config.api.transactionPort}`);

/**
 * API for Organizations
 */
export const organizationAPI = {
  // Create an organization
  create: async (data) => {
    try {
      console.log('Creating organization with data:', JSON.stringify(data, null, 2));
      const response = await onboardingClient.post('/v1/organizations', data);
      return response.data;
    } catch (error) {
      console.error('Error creating organization:', error.message);
      throw error;
    }
  },

  // Get an organization by ID
  get: async (id) => {
    try {
      const response = await onboardingClient.get(`/v1/organizations/${id}`);
      return response.data;
    } catch (error) {
      console.error(`Error getting organization ${id}:`, error.message);
      throw error;
    }
  },

  // Update an organization
  update: async (id, data) => {
    try {
      const response = await onboardingClient.patch(`/v1/organizations/${id}`, data);
      return response.data;
    } catch (error) {
      console.error(`Error updating organization ${id}:`, error.message);
      throw error;
    }
  },

  // List all organizations
  list: async (params = {}) => {
    try {
      const response = await onboardingClient.get('/v1/organizations', { params });
      return response.data;
    } catch (error) {
      console.error('Error listing organizations:', error.message);
      throw error;
    }
  }
};

/**
 * API for Ledgers
 */
export const ledgerAPI = {
  // Create a ledger
  create: async (organizationId, data) => {
    try {
      const response = await onboardingClient.post(`/v1/organizations/${organizationId}/ledgers`, data);
      return response.data;
    } catch (error) {
      console.error(`Error creating ledger for organization ${organizationId}:`, error.message);
      throw error;
    }
  },

  // Get a ledger by ID
  get: async (organizationId, ledgerId) => {
    try {
      const response = await onboardingClient.get(`/v1/organizations/${organizationId}/ledgers/${ledgerId}`);
      return response.data;
    } catch (error) {
      console.error(`Error getting ledger ${ledgerId}:`, error.message);
      throw error;
    }
  },

  // Update a ledger
  update: async (organizationId, ledgerId, data) => {
    try {
      const response = await onboardingClient.patch(`/v1/organizations/${organizationId}/ledgers/${ledgerId}`, data);
      return response.data;
    } catch (error) {
      console.error(`Error updating ledger ${ledgerId}:`, error.message);
      throw error;
    }
  },

  // List all ledgers for an organization
  list: async (organizationId, params = {}) => {
    try {
      const response = await onboardingClient.get(`/v1/organizations/${organizationId}/ledgers`, { params });
      return response.data;
    } catch (error) {
      console.error(`Error listing ledgers for organization ${organizationId}:`, error.message);
      throw error;
    }
  }
};

/**
 * API for Assets
 */
export const assetAPI = {
  // Create an asset
  create: async (organizationId, ledgerId, data) => {
    try {
      const response = await onboardingClient.post(`/v1/organizations/${organizationId}/ledgers/${ledgerId}/assets`, data);
      return response.data;
    } catch (error) {
      console.error(`Error creating asset in ledger ${ledgerId}:`, error.message);
      throw error;
    }
  },

  // Get an asset by ID
  get: async (organizationId, ledgerId, assetId) => {
    try {
      const response = await onboardingClient.get(`/v1/organizations/${organizationId}/ledgers/${ledgerId}/assets/${assetId}`);
      return response.data;
    } catch (error) {
      console.error(`Error getting asset ${assetId}:`, error.message);
      throw error;
    }
  },

  // Update an asset
  update: async (organizationId, ledgerId, assetId, data) => {
    try {
      const response = await onboardingClient.patch(`/v1/organizations/${organizationId}/ledgers/${ledgerId}/assets/${assetId}`, data);
      return response.data;
    } catch (error) {
      console.error(`Error updating asset ${assetId}:`, error.message);
      throw error;
    }
  },

  // List all assets for a ledger
  list: async (organizationId, ledgerId, params = {}) => {
    try {
      const response = await onboardingClient.get(`/v1/organizations/${organizationId}/ledgers/${ledgerId}/assets`, { params });
      return response.data;
    } catch (error) {
      console.error(`Error listing assets for ledger ${ledgerId}:`, error.message);
      throw error;
    }
  }
};

/**
 * API for Accounts
 */
export const accountAPI = {
  // Create an account
  create: async (organizationId, ledgerId, data) => {
    try {
      const response = await onboardingClient.post(`/v1/organizations/${organizationId}/ledgers/${ledgerId}/accounts`, data);
      return response.data;
    } catch (error) {
      console.error(`Error creating account in ledger ${ledgerId}:`, error.message);
      throw error;
    }
  },

  // Get an account by ID
  get: async (organizationId, ledgerId, accountId) => {
    try {
      const response = await onboardingClient.get(`/v1/organizations/${organizationId}/ledgers/${ledgerId}/accounts/${accountId}`);
      return response.data;
    } catch (error) {
      console.error(`Error getting account ${accountId}:`, error.message);
      throw error;
    }
  },

  // Update an account
  update: async (organizationId, ledgerId, accountId, data) => {
    try {
      const response = await onboardingClient.patch(`/v1/organizations/${organizationId}/ledgers/${ledgerId}/accounts/${accountId}`, data);
      return response.data;
    } catch (error) {
      console.error(`Error updating account ${accountId}:`, error.message);
      throw error;
    }
  },

  // List all accounts for a ledger
  list: async (organizationId, ledgerId, params = {}) => {
    try {
      const response = await onboardingClient.get(`/v1/organizations/${organizationId}/ledgers/${ledgerId}/accounts`, { params });
      return response.data;
    } catch (error) {
      console.error(`Error listing accounts for ledger ${ledgerId}:`, error.message);
      throw error;
    }
  }
};

/**
 * API for Segments
 */
export const segmentAPI = {
  // Create a segment
  create: async (organizationId, ledgerId, data) => {
    try {
      const response = await onboardingClient.post(`/v1/organizations/${organizationId}/ledgers/${ledgerId}/segments`, data);
      return response.data;
    } catch (error) {
      console.error(`Error creating segment in ledger ${ledgerId}:`, error.message);
      throw error;
    }
  },

  // Get a segment by ID
  get: async (organizationId, ledgerId, segmentId) => {
    try {
      const response = await onboardingClient.get(`/v1/organizations/${organizationId}/ledgers/${ledgerId}/segments/${segmentId}`);
      return response.data;
    } catch (error) {
      console.error(`Error getting segment ${segmentId}:`, error.message);
      throw error;
    }
  },

  // Update a segment
  update: async (organizationId, ledgerId, segmentId, data) => {
    try {
      const response = await onboardingClient.patch(`/v1/organizations/${organizationId}/ledgers/${ledgerId}/segments/${segmentId}`, data);
      return response.data;
    } catch (error) {
      console.error(`Error updating segment ${segmentId}:`, error.message);
      throw error;
    }
  },

  // List all segments for a ledger
  list: async (organizationId, ledgerId, params = {}) => {
    try {
      const response = await onboardingClient.get(`/v1/organizations/${organizationId}/ledgers/${ledgerId}/segments`, { params });
      return response.data;
    } catch (error) {
      console.error(`Error listing segments for ledger ${ledgerId}:`, error.message);
      throw error;
    }
  }
};

/**
 * API for Portfolios
 */
export const portfolioAPI = {
  // Create a portfolio
  create: async (organizationId, ledgerId, data) => {
    try {
      const response = await onboardingClient.post(`/v1/organizations/${organizationId}/ledgers/${ledgerId}/portfolios`, data);
      return response.data;
    } catch (error) {
      console.error(`Error creating portfolio in ledger ${ledgerId}:`, error.message);
      throw error;
    }
  },

  // Get a portfolio by ID
  get: async (organizationId, ledgerId, portfolioId) => {
    try {
      const response = await onboardingClient.get(`/v1/organizations/${organizationId}/ledgers/${ledgerId}/portfolios/${portfolioId}`);
      return response.data;
    } catch (error) {
      console.error(`Error getting portfolio ${portfolioId}:`, error.message);
      throw error;
    }
  },

  // Update a portfolio
  update: async (organizationId, ledgerId, portfolioId, data) => {
    try {
      const response = await onboardingClient.patch(`/v1/organizations/${organizationId}/ledgers/${ledgerId}/portfolios/${portfolioId}`, data);
      return response.data;
    } catch (error) {
      console.error(`Error updating portfolio ${portfolioId}:`, error.message);
      throw error;
    }
  },

  // List all portfolios for a ledger
  list: async (organizationId, ledgerId, params = {}) => {
    try {
      const response = await onboardingClient.get(`/v1/organizations/${organizationId}/ledgers/${ledgerId}/portfolios`, { params });
      return response.data;
    } catch (error) {
      console.error(`Error listing portfolios for ledger ${ledgerId}:`, error.message);
      throw error;
    }
  }
};

/**
 * API for Transactions
 */
export const transactionAPI = {
  // Create a transaction
  create: async (organizationId, ledgerId, data) => {
    try {
      // Add idempotency key to ensure the transaction is not duplicated
      const idempotencyKey = uuidv4();
      const headers = { 'Idempotency-Key': idempotencyKey };
      
      const response = await transactionClient.post(
        `/v1/organizations/${organizationId}/ledgers/${ledgerId}/transactions/json`, 
        data,
        { headers }
      );
      return response.data;
    } catch (error) {
      console.error(`Error creating transaction in ledger ${ledgerId}:`, error.message);
      throw error;
    }
  },

  // Get a transaction by ID
  get: async (organizationId, ledgerId, transactionId) => {
    try {
      const response = await transactionClient.get(`/v1/organizations/${organizationId}/ledgers/${ledgerId}/transactions/${transactionId}`);
      return response.data;
    } catch (error) {
      console.error(`Error getting transaction ${transactionId}:`, error.message);
      throw error;
    }
  },

  // Update a transaction (metadata)
  update: async (organizationId, ledgerId, transactionId, data) => {
    try {
      const response = await transactionClient.patch(`/v1/organizations/${organizationId}/ledgers/${ledgerId}/transactions/${transactionId}`, data);
      return response.data;
    } catch (error) {
      console.error(`Error updating transaction ${transactionId}:`, error.message);
      throw error;
    }
  },

  // List all transactions for a ledger
  list: async (organizationId, ledgerId, params = {}) => {
    try {
      const response = await transactionClient.get(`/v1/organizations/${organizationId}/ledgers/${ledgerId}/transactions`, { params });
      return response.data;
    } catch (error) {
      console.error(`Error listing transactions for ledger ${ledgerId}:`, error.message);
      throw error;
    }
  }
};

// Export all API clients
export default {
  organizationAPI,
  ledgerAPI,
  assetAPI,
  accountAPI,
  segmentAPI,
  portfolioAPI,
  transactionAPI
};