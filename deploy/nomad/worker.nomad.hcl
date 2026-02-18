job "penfold-worker" {
  datacenters = ["dc1"]
  type        = "service"

  constraint {
    attribute = "${meta.apple_silicon}"
    value     = "true"
  }

  update {
    max_parallel     = 1
    canary           = 0
    min_healthy_time = "10s"
    healthy_deadline = "3m"
    auto_revert      = true
  }

  group "worker" {
    count = 1

    network {
      port "http" {
        static       = 8085
        host_network = "default"
      }
    }

    restart {
      attempts = 3
      delay    = "30s"
      interval = "5m"
      mode     = "fail"
    }

    task "worker" {
      driver = "raw_exec"

      shutdown_delay = "5s"
      kill_signal    = "SIGTERM"
      kill_timeout   = "35s"

      config {
        command = "/bin/sh"
        args    = ["-c", "set -a; . /etc/penfold/worker.env; set +a; exec /opt/penfold/bin/penfold-worker"]
      }

      service {
        name     = "penfold-worker"
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

      resources {
        cpu    = 1000
        memory = 2048
      }
    }
  }
}
