# Prometheus Metrics

This file lists all metrics currently available in Prometheus as part of the LGTM stack.
Last updated: Tue Mar 18 19:06:25 -03 2025

## Business Metrics
- `business_balance_count_total` 
- `business_balance_duration_milliseconds_bucket` 
- `business_balance_duration_milliseconds_count` 
- `business_balance_duration_milliseconds_sum` 
- `business_onboarding_count_total` 
- `business_onboarding_duration_milliseconds_bucket` 
- `business_onboarding_duration_milliseconds_count` 
- `business_onboarding_duration_milliseconds_sum` 
- `business_transaction_count_total` 
- `business_transaction_duration_milliseconds_bucket` 
- `business_transaction_duration_milliseconds_count` 
- `business_transaction_duration_milliseconds_sum` 

## HTTP Server Metrics
- `http_server_duration_milliseconds_bucket` 
- `http_server_duration_milliseconds_count` 
- `http_server_duration_milliseconds_sum` 
- `http_server_requests_total` 

## OpenTelemetry Collector Metrics
- `otelcol_exporter_queue_capacity` 
- `otelcol_exporter_queue_size` 
- `otelcol_exporter_send_failed_log_records_total` 
- `otelcol_exporter_send_failed_metric_points_total` 
- `otelcol_exporter_send_failed_spans_total` 
- `otelcol_exporter_sent_log_records_total` 
- `otelcol_exporter_sent_metric_points_total` 
- `otelcol_exporter_sent_spans_total` 
- `otelcol_process_cpu_seconds_total` 
- `otelcol_process_memory_rss` 
- `otelcol_process_runtime_alloc_bytes_total` 
- `otelcol_process_runtime_heap_alloc_bytes` 
- `otelcol_process_runtime_total_sys_memory_bytes` 
- `otelcol_process_uptime_total` 
- `otelcol_processor_batch_batch_send_size_bucket` 
- `otelcol_processor_batch_batch_send_size_count` 
- `otelcol_processor_batch_batch_send_size_sum` 
- `otelcol_processor_batch_metadata_cardinality` 
- `otelcol_processor_batch_timeout_trigger_send_total` 
- `otelcol_receiver_accepted_log_records_total` 
- `otelcol_receiver_accepted_metric_points_total` 
- `otelcol_receiver_accepted_spans_total` 
- `otelcol_receiver_refused_log_records_total` 
- `otelcol_receiver_refused_metric_points_total` 
- `otelcol_receiver_refused_spans_total` 

## Prometheus Internal Metrics
- `promhttp_metric_handler_errors_total` 
- `scrape_duration_seconds` 
- `scrape_samples_post_metric_relabeling` 
- `scrape_samples_scraped` 
- `scrape_series_added` 
- `up` 

## System and Service Metrics
- `service_metadata_info_instance` 
- `system_resource_usage_cpu_percentage` 
- `system_resource_usage_memory_percentage` 
- `target_info` 

## Tracing Metrics
- `traces_spanmetrics_calls_total` 
- `traces_spanmetrics_latency_bucket` 
- `traces_spanmetrics_latency_count` 
- `traces_spanmetrics_latency_sum` 
- `traces_spanmetrics_size_total` 

