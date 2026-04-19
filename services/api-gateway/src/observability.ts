interface RequestMetric {
  method: string;
  route: string;
  statusCode: string;
  durationMs: number;
}

const requestMetrics: RequestMetric[] = [];
const fraudCounters = { ok: 0, error: 0 };

export function recordRequestDuration(method: string, route: string, statusCode: string, durationMs: number): void {
  requestMetrics.push({ method, route, statusCode, durationMs });
  if (requestMetrics.length > 5000) {
    requestMetrics.shift();
  }
}

export function incrementFraudCounter(status: "ok" | "error"): void {
  fraudCounters[status] += 1;
}

export function renderPrometheusMetrics(): string {
  const lines = [
    "# HELP caas_api_gateway_fraud_requests_total Outbound calls to fraud-pipeline",
    "# TYPE caas_api_gateway_fraud_requests_total counter",
    `caas_api_gateway_fraud_requests_total{status=\"ok\"} ${fraudCounters.ok}`,
    `caas_api_gateway_fraud_requests_total{status=\"error\"} ${fraudCounters.error}`,
    "# HELP caas_api_gateway_http_request_duration_ms Recent request durations in milliseconds",
    "# TYPE caas_api_gateway_http_request_duration_ms gauge",
  ];

  for (const metric of requestMetrics.slice(-200)) {
    lines.push(
      `caas_api_gateway_http_request_duration_ms{method=\"${metric.method}\",route=\"${metric.route}\",status_code=\"${metric.statusCode}\"} ${metric.durationMs}`,
    );
  }

  return `${lines.join("\n")}\n`;
}

export const metricsContentType = "text/plain; version=0.0.4";
