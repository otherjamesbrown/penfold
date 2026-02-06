job "penfold-mlx" {
  datacenters = ["dc1"]
  type        = "service"

  constraint {
    attribute = "${meta.apple-silicon}"
    value     = "true"
  }

  # --- MLX Embeddings Server ---
  # Model: mxbai-embed-large-v1 (1024 dimensions)
  # Always running; used by all enrichment pipelines.
  group "embeddings" {
    count = 1

    network {
      port "http" { static = 8081 }
    }

    restart {
      attempts = 3
      delay    = "30s"
      interval = "5m"
      mode     = "fail"
    }

    task "embeddings" {
      driver = "raw_exec"

      config {
        command = "/bin/sh"
        args = [
          "-c",
          "cd /Users/james/github/otherjamesbrown/penfold/penfold-go-pipeline/sidecar && exec .venv/bin/uvicorn app:app --host 0.0.0.0 --port 8081",
        ]
      }

      env {
        PATH = "/Users/james/github/otherjamesbrown/penfold/penfold-go-pipeline/sidecar/.venv/bin:/usr/local/bin:/usr/bin:/bin"
      }

      service {
        name = "penfold-mlx-embeddings"
        port = "http"

        check {
          name     = "http-health"
          type     = "http"
          path     = "/health"
          interval = "10s"
          timeout  = "5s"
        }
      }

      resources {
        cpu    = 1000
        memory = 4096
      }
    }
  }

  # --- MLX LLM Server ---
  # Model: Qwen2.5-7B-Instruct-4bit (fallback LLM; primary is Gemini via AI Coordinator)
  group "llm" {
    count = 1

    network {
      port "http" { static = 8080 }
    }

    restart {
      attempts = 3
      delay    = "30s"
      interval = "5m"
      mode     = "fail"
    }

    task "llm" {
      driver = "raw_exec"

      config {
        command = "/bin/sh"
        args = [
          "-c",
          "exec /Users/james/github/otherjamesbrown/penfold/penfold-go-pipeline/sidecar/.venv/bin/mlx_lm.server --model mlx-community/Qwen2.5-7B-Instruct-4bit --port 8080 --host 0.0.0.0",
        ]
      }

      service {
        name = "penfold-mlx-llm"
        port = "http"

        check {
          name     = "http-health"
          type     = "http"
          path     = "/health"
          interval = "10s"
          timeout  = "5s"
        }
      }

      resources {
        cpu    = 2000
        memory = 8192
      }
    }
  }

  # --- MLX LM Exporter ---
  # Prometheus metrics exporter for MLX services.
  group "exporter" {
    count = 1

    network {
      port "http" { static = 9101 }
    }

    restart {
      attempts = 3
      delay    = "30s"
      interval = "5m"
      mode     = "fail"
    }

    task "exporter" {
      driver = "raw_exec"

      config {
        command = "/bin/sh"
        args = [
          "-c",
          "exec /usr/bin/python3 /Users/james/github/otherjamesbrown/mlx-lm-exporter/mlx_lm_exporter.py --mlx-server http://localhost:8080 --port 9101",
        ]
      }

      service {
        name = "penfold-mlx-exporter"
        port = "http"

        check {
          name     = "http-health"
          type     = "http"
          path     = "/metrics"
          interval = "15s"
          timeout  = "5s"
        }
      }

      resources {
        cpu    = 200
        memory = 256
      }
    }
  }
}
