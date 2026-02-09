job "penfold-mlx" {
  datacenters = ["dc1"]
  type        = "service"

  constraint {
    attribute = "${meta.apple_silicon}"
    value     = "true"
  }

  # --- MLX Embeddings Server ---
  # Model: mxbai-embed-large-v1 (1024 dimensions)
  # Always running; used by all enrichment pipelines.
  group "embeddings" {
    count = 1

    network {
      port "http" {
        static       = 8081
        host_network = "default"
      }
    }

    restart {
      attempts = 5
      delay    = "30s"
      interval = "10m"
      mode     = "fail"
    }

    task "embeddings" {
      driver = "raw_exec"

      kill_signal  = "SIGTERM"
      kill_timeout = "10s"

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
      port "http" {
        static       = 8080
        host_network = "default"
      }
    }

    restart {
      attempts = 5
      delay    = "30s"
      interval = "10m"
      mode     = "fail"
    }

    task "llm" {
      driver = "raw_exec"

      kill_signal  = "SIGTERM"
      kill_timeout = "10s"

      config {
        command = "/bin/sh"
        args = [
          "-c",
          "exec /Users/james/github/otherjamesbrown/penfold/penfold-go-pipeline/sidecar/.venv/bin/mlx_lm.server --model mlx-community/Qwen2.5-7B-Instruct-4bit --port 8080 --host 0.0.0.0",
        ]
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
      port "http" {
        static       = 9101
        host_network = "default"
      }
    }

    restart {
      attempts = 5
      delay    = "30s"
      interval = "10m"
      mode     = "fail"
    }

    task "exporter" {
      driver = "raw_exec"

      kill_signal  = "SIGTERM"
      kill_timeout = "10s"

      config {
        command = "/bin/sh"
        args = [
          "-c",
          "exec /Users/james/github/otherjamesbrown/penfold/penfold-go-pipeline/sidecar/.venv/bin/python3 /Users/james/github/otherjamesbrown/mlx-lm-exporter/mlx_lm_exporter.py --mlx-server http://localhost:8080 --port 9101",
        ]
      }

      resources {
        cpu    = 200
        memory = 256
      }
    }
  }
}
