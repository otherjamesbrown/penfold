job "penfold-ai-coordinator" {
  datacenters = ["dc1"]
  type        = "service"

  constraint {
    attribute = "${meta.os}"
    value     = "linux"
  }

  update {
    max_parallel     = 1
    min_healthy_time = "10s"
    healthy_deadline = "90s"
    auto_revert      = true
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
      user   = "james"

      kill_signal  = "SIGTERM"
      kill_timeout = "35s"

      config {
        command = "/bin/sh"
        args    = ["-c", "set -a; . /etc/penfold/ai-coordinator.env; set +a; exec /opt/penfold/bin/penfold-ai-coordinator"]
      }

      service {
        name     = "penfold-ai-coordinator"
        port     = "http"
        provider = "nomad"

        check {
          name     = "http-health"
          type     = "http"
          path     = "/health"
          interval = "10s"
          timeout  = "3s"
        }

        check_restart {
          limit           = 3
          grace           = "30s"
          ignore_warnings = false
        }
      }

      service {
        name     = "penfold-ai-coordinator-grpc"
        port     = "grpc"
        provider = "nomad"

        check {
          name     = "grpc-health"
          type     = "tcp"
          interval = "10s"
          timeout  = "3s"
        }

        check_restart {
          limit           = 3
          grace           = "30s"
          ignore_warnings = false
        }
      }

      resources {
        cpu    = 500
        memory = 512
      }
    }
  }
}
