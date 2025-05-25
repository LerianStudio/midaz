/**
 * Worker pool utility for concurrent processing
 */

export interface WorkerPoolOptions {
  concurrency?: number;
  preserveOrder?: boolean;
  continueOnError?: boolean;
}

export async function workerPool<T, R>(
  items: T[],
  processor: (item: T) => Promise<R>,
  optionsOrConcurrency: number | WorkerPoolOptions = 10
): Promise<R[]> {
  const options: WorkerPoolOptions = typeof optionsOrConcurrency === 'number' 
    ? { concurrency: optionsOrConcurrency }
    : optionsOrConcurrency;
  
  const maxConcurrency = options.concurrency || 10;
  const results: R[] = [];
  const errors: Error[] = [];
  
  // Process items in batches
  for (let i = 0; i < items.length; i += maxConcurrency) {
    const batch = items.slice(i, i + maxConcurrency);
    const batchResults = await Promise.allSettled(
      batch.map(item => processor(item))
    );
    
    batchResults.forEach((result, index) => {
      if (result.status === 'fulfilled') {
        results.push(result.value);
      } else {
        errors.push(new Error(`Failed to process item ${i + index}: ${result.reason}`));
      }
    });
  }
  
  if (!options.continueOnError && errors.length > 0) {
    throw errors[0];
  }
  
  if (errors.length > 0 && errors.length === items.length) {
    throw new Error(`All items failed to process: ${errors[0].message}`);
  }
  
  return results;
}