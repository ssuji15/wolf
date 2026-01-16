otelcol.exporter.otlp "tempo" {
  client {
    endpoint = env("TEMPO_OTLP_ENDPOINT")
    tls {
      insecure = true
    }
  }
}

otelcol.receiver.otlp "default" {
  http {
    endpoint = "0.0.0.0:8085"
  }
  grpc {
    endpoint = "0.0.0.0:8086"
  }

  output {
    traces = [
      otelcol.connector.spanmetrics.traces_to_metrics.input,
      otelcol.exporter.otlp.tempo.input,
    ]

    metrics = [
      otelcol.processor.batch.metrics_batcher.input,
    ]
  }
}

logging {
  level = "debug"
  format = "json"
}

prometheus.remote_write "default" {
  endpoint {
    url = env("PROM_REMOTE_WRITE_URL")
  }
}

otelcol.exporter.prometheus "default" {
  forward_to = [prometheus.remote_write.default.receiver]
}

otelcol.processor.batch "metrics_batcher" {
  timeout = "5s"
  output {
    metrics = [
      otelcol.exporter.prometheus.default.input,
    ]
  }
}

otelcol.connector.spanmetrics "traces_to_metrics" {
    histogram {
       explicit {
         buckets = ["2ms", "4ms", "8ms", "16ms", "32ms", "50ms", "75ms", "100ms", "125ms", "150ms", "175ms", "200ms", "250ms", "300ms", "350ms", "400ms", "450ms", "500ms", "600ms", "700ms", "800ms", "900ms", "1s"]
       }
    }
    dimension {
        name = "service.name"
    }
    dimension {
        name = "span.name"
    }
    dimension {
        name = "span.kind"
    }

    output {
      metrics = [otelcol.exporter.prometheus.default.input]
    }
}