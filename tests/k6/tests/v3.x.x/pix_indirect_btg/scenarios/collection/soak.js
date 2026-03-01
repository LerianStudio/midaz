/**
 * PIX Collection Soak Test
 *
 * CRITICAL: Minimum 4 hours duration (BACEN requirement for PIX systems)
 *
 * Extended duration test to detect:
 * - Memory leaks
 * - Resource exhaustion
 * - Connection pool issues
 * - Database connection leaks
 * - Gradual performance degradation
 *
 * Default: 4 hours with realistic load variation
 * Load Pattern: Realistic variation simulating business day
 * Purpose: Ensure system stability over extended periods
 *
 * Dynamic Parameters (override via environment variables):
 *   - DURATION: Test duration (e.g., '1h', '4h', '8h')
 *   - MIN_VUS: Minimum preAllocated VUs
 *   - MAX_VUS: Maximum VUs for arrival rate scenarios
 *
 * Usage:
 *   k6 run soak.js -e DURATION=30m          # Short soak for dev testing
 *   k6 run soak.js -e DURATION=4h           # Full BACEN compliance
 *   k6 run soak.js -e DURATION=2h -e MAX_VUS=300
 */

import * as auth from '../../../../../pkg/auth.js';
import * as thresholds from '../../config/thresholds.js';
import { createCollectionFlow } from '../../flows/create-collection-flow.js';
import { Trend } from 'k6/metrics';
import { textSummary } from 'https://jslib.k6.io/k6-summary/0.0.4/index.js';

// Custom metrics for degradation analysis
const hourlyLatency = new Trend('pix_hourly_latency', true);

// Think time mode
const THINK_TIME_MODE = __ENV.K6_THINK_TIME_MODE || 'realistic';
const TOKEN_VALIDITY_MS = 55 * 60 * 1000;
const TOKEN_REFRESH_THRESHOLD_MS = 5 * 60 * 1000;

let cachedToken = null;
let cachedTokenExpiry = 0;

function getValidToken(data) {
  const now = Date.now();

  if (cachedToken && cachedTokenExpiry > now + TOKEN_REFRESH_THRESHOLD_MS) {
    return cachedToken;
  }

  if (data.token && data.tokenExpiry && data.tokenExpiry > now + TOKEN_REFRESH_THRESHOLD_MS) {
    return data.token;
  }

  cachedToken = auth.generateToken();
  cachedTokenExpiry = now + TOKEN_VALIDITY_MS;
  return cachedToken;
}

export const options = {
  scenarios: {
    collection_soak: {
      exec: 'collectionSoak',
      ...thresholds.getScenario('soak')
    }
  },
  thresholds: thresholds.getThresholds('soak')
};

export function setup() {
  const token = auth.generateToken();
  const accounts = JSON.parse(open('../../data/accounts.json'));
  const dynamicInfo = thresholds.dynamicParams;

  console.log('\n' + '='.repeat(70));
  console.log('         PIX COLLECTION SOAK TEST');
  console.log('         BACEN REGULATORY REQUIREMENT');
  console.log('='.repeat(70));
  console.log(`Duration: ${dynamicInfo.duration || '4h (default)'}`);
  console.log(`Min VUs: ${dynamicInfo.minVUs || '100 (default)'}`);
  console.log(`Max VUs: ${dynamicInfo.maxVUs || '200 (default)'}`);
  console.log(`Load Pattern: Realistic (ramping with variations)`);
  console.log(`Think Time Mode: ${THINK_TIME_MODE}`);
  console.log(`Accounts available: ${accounts.length}`);
  console.log('');
  console.log('LOAD PATTERN:');
  console.log('  Hour 1: Morning ramp-up (20 -> 100 RPS)');
  console.log('  Hour 2: Peak load (100 -> 150 RPS)');
  console.log('  Hour 3: Afternoon sustained (100-110 RPS)');
  console.log('  Hour 4: Evening wind-down (80 -> 20 RPS)');
  console.log('');
  console.log('MONITORING POINTS:');
  console.log('  - Latency degradation (compare first vs last hour)');
  console.log('  - Technical error rate (must be < 0.5%)');
  console.log('  - Business error rate (acceptable up to 5%)');
  console.log('  - Memory/resource stability');
  console.log('='.repeat(70) + '\n');

  return {
    token,
    tokenExpiry: Date.now() + TOKEN_VALIDITY_MS,
    accounts,
    startTime: Date.now()
  };
}

export function collectionSoak(data) {
  const accountIndex = __VU % data.accounts.length;
  const account = data.accounts[accountIndex];

  const startTime = Date.now();
  const token = getValidToken(data);

  createCollectionFlow({
    token,
    accountId: account.id,
    receiverDocument: account.document
  });

  const duration = Date.now() - startTime;

  // Track hourly latency for degradation analysis
  // Tag by which hour of the test we're in
  const elapsedHours = Math.floor((Date.now() - data.startTime) / (1000 * 60 * 60));
  hourlyLatency.add(duration, { hour: String(elapsedHours) });
}

export function handleSummary(data) {
  const durationMs = data.state.testRunDurationMs;
  const durationHours = (durationMs / 1000 / 60 / 60).toFixed(2);
  const totalIterations = data.metrics.iterations?.values?.count || 0;
  const avgRps = totalIterations / (durationMs / 1000);

  // Calculate performance stability metrics
  const avgLatency = data.metrics.pix_collection_create_duration?.values?.avg || 0;
  const p95Latency = data.metrics.pix_collection_create_duration?.values?.['p(95)'] || 0;
  const p99Latency = data.metrics.pix_collection_create_duration?.values?.['p(99)'] || 0;
  const minLatency = data.metrics.pix_collection_create_duration?.values?.min || 0;
  const maxLatency = data.metrics.pix_collection_create_duration?.values?.max || 0;

  // Error metrics
  const collectionErrorRate = (data.metrics.pix_collection_error_rate?.values?.rate || 0) * 100;
  const technicalErrorRate = (data.metrics.pix_technical_error_rate?.values?.rate || 0) * 100;
  const businessErrorRate = (data.metrics.pix_business_error_rate?.values?.rate || 0) * 100;

  // Check for performance degradation
  // Max should not be too far from p99 (indicates outliers/degradation)
  const degradationRatio = p99Latency > 0 ? maxLatency / p99Latency : 0;
  const isLatencyStable = degradationRatio < 5; // Max should be < 5x P99

  // Variance check - high variance indicates instability
  const latencyVariance = maxLatency - minLatency;
  const isVarianceAcceptable = latencyVariance < (avgLatency * 10); // Variance < 10x avg

  console.log('\n' + '='.repeat(70));
  console.log('                    SOAK TEST SUMMARY');
  console.log('                    BACEN COMPLIANCE CHECK');
  console.log('='.repeat(70));
  console.log(`Duration: ${durationHours} hours`);
  console.log(`Total Iterations: ${totalIterations.toLocaleString()}`);
  console.log(`Average RPS: ${avgRps.toFixed(2)}`);

  console.log('-'.repeat(70));
  console.log('COLLECTIONS');
  console.log(`  Created: ${data.metrics.pix_collection_created?.values?.count || 0}`);
  console.log(`  Failed: ${data.metrics.pix_collection_failed?.values?.count || 0}`);
  console.log(`  Duplicates: ${data.metrics.pix_collection_duplicate_txid?.values?.count || 0}`);

  console.log('-'.repeat(70));
  console.log('ERROR ANALYSIS (SEGREGATED)');
  console.log(`  Overall Error Rate: ${collectionErrorRate.toFixed(4)}%`);
  console.log(`  Technical Errors (5xx, timeouts): ${technicalErrorRate.toFixed(4)}%`);
  console.log(`  Business Errors (validation, rules): ${businessErrorRate.toFixed(4)}%`);
  console.log(`  Technical Error Count: ${data.metrics.pix_technical_errors?.values?.count || 0}`);
  console.log(`  Business Error Count: ${data.metrics.pix_business_errors?.values?.count || 0}`);

  console.log('-'.repeat(70));
  console.log('LATENCY DISTRIBUTION');
  console.log(`  Min: ${Math.round(minLatency)}ms`);
  console.log(`  Avg: ${Math.round(avgLatency)}ms`);
  console.log(`  P95: ${Math.round(p95Latency)}ms`);
  console.log(`  P99: ${Math.round(p99Latency)}ms`);
  console.log(`  Max: ${Math.round(maxLatency)}ms`);

  console.log('-'.repeat(70));
  console.log('DEGRADATION ANALYSIS');
  console.log(`  Degradation Ratio (Max/P99): ${degradationRatio.toFixed(2)}x ${isLatencyStable ? '(OK)' : '(HIGH!)'}`);
  console.log(`  Latency Variance: ${Math.round(latencyVariance)}ms ${isVarianceAcceptable ? '(OK)' : '(HIGH!)'}`);
  console.log(`  System Stability: ${isLatencyStable && isVarianceAcceptable ? 'STABLE' : 'DEGRADATION DETECTED'}`);

  console.log('-'.repeat(70));
  console.log('BACEN COMPLIANCE');
  const durationOk = parseFloat(durationHours) >= 4;
  const collectionErrorOk = collectionErrorRate < 2;
  const technicalErrorOk = technicalErrorRate < 0.5; // Stricter for technical errors
  const latencyOk = p95Latency < 800;
  const stabilityOk = isLatencyStable && isVarianceAcceptable;

  console.log(`  Duration >= 4h: ${durationOk ? 'PASS' : 'FAIL'}`);
  console.log(`  Overall Error Rate < 2%: ${collectionErrorOk ? 'PASS' : 'FAIL'}`);
  console.log(`  Technical Error Rate < 0.5%: ${technicalErrorOk ? 'PASS' : 'FAIL'}`);
  console.log(`  P95 Latency < 800ms: ${latencyOk ? 'PASS' : 'FAIL'}`);
  console.log(`  System Stability: ${stabilityOk ? 'PASS' : 'FAIL'}`);

  const overallCompliant = durationOk && collectionErrorOk && technicalErrorOk && latencyOk && stabilityOk;
  console.log(`  Overall: ${overallCompliant ? 'COMPLIANT' : 'NON-COMPLIANT'}`);

  if (!overallCompliant) {
    console.log('-'.repeat(70));
    console.log('FAILURE REASONS:');
    if (!durationOk) console.log('  - Test duration < 4 hours');
    if (!collectionErrorOk) console.log('  - Overall error rate >= 2%');
    if (!technicalErrorOk) console.log('  - Technical error rate >= 0.5% (system failures)');
    if (!latencyOk) console.log('  - P95 latency >= 800ms');
    if (!stabilityOk) console.log('  - System showed degradation over time');
  }

  console.log('='.repeat(70) + '\n');

  // Return JSON summary for programmatic access
  return {
    stdout: textSummary(data, { indent: ' ', enableColors: true }),
    'soak-summary.json': JSON.stringify({
      duration: {
        ms: durationMs,
        hours: parseFloat(durationHours)
      },
      iterations: totalIterations,
      rps: parseFloat(avgRps.toFixed(2)),
      errors: {
        overall: parseFloat(collectionErrorRate.toFixed(4)),
        technical: parseFloat(technicalErrorRate.toFixed(4)),
        business: parseFloat(businessErrorRate.toFixed(4))
      },
      latency: {
        min: Math.round(minLatency),
        avg: Math.round(avgLatency),
        p95: Math.round(p95Latency),
        p99: Math.round(p99Latency),
        max: Math.round(maxLatency)
      },
      stability: {
        degradationRatio: parseFloat(degradationRatio.toFixed(2)),
        variance: Math.round(latencyVariance),
        isStable: stabilityOk
      },
      compliance: {
        durationOk,
        errorRateOk: collectionErrorOk,
        technicalErrorOk,
        latencyOk,
        stabilityOk,
        overall: overallCompliant
      }
    }, null, 2)
  };
}
