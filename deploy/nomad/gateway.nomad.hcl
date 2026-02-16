job "penfold-gateway" {
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

  group "gateway" {
    count = 1

    network {
      port "grpc" { static = 50051 }
      port "http" { static = 8080 }
    }

    restart {
      attempts = 3
      delay    = "30s"
      interval = "5m"
      mode     = "fail"
    }

    task "gateway" {
      driver = "raw_exec"
      user   = "james"

      kill_signal  = "SIGTERM"
      kill_timeout = "35s"

      config {
        command = "/bin/sh"
        args    = ["-c", "set -a; . /etc/penfold/gateway.env; set +a; exec /opt/penfold/bin/penfold-gateway"]
      }

      service {
        name     = "penfold-gateway"
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
        name     = "penfold-gateway-grpc"
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
