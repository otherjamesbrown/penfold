job "penfold-ai-coordinator" {
  datacenters = ["dc1"]
  type        = "service"

  constraint {
    attribute = "${meta.os}"
    value     = "linux"
  }

  update {
    max_parallel     = 1
    canary           = 1
    min_healthy_time = "10s"
    healthy_deadline = "60s"
    auto_revert      = true
    auto_promote     = true
  }

  group "ai-coordinator" {
    count = 1

    network {
      port "grpc" { static = 50055 }
      port "http" { static = 8090 }
    }

    restart {
      attempts = 3
      delay    = "30s"
      interval = "5m"
      mode     = "fail"
    }

    task "ai-coordinator" {
      driver = "raw_exec"

      config {
        command = "/bin/sh"
        args    = ["-c", "set -a; . /etc/penfold/ai-coordinator.env; set +a; exec /opt/penfold/bin/penfold-ai-coordinator"]
      }

      service {
        name = "penfold-ai-coordinator"
        port = "http"

        check {
          name     = "http-health"
          type     = "http"
          path     = "/health"
          interval = "10s"
          timeout  = "3s"
        }
      }

      service {
        name = "penfold-ai-coordinator-grpc"
        port = "grpc"
      }

      resources {
        cpu    = 500
        memory = 512
      }
    }
  }
}
