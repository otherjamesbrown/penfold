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

    # Kill orphan workers before the main task starts. On macOS, raw_exec
    # through sudo doesn't forward SIGTERM reliably, so previous workers
    # can survive Nomad restarts. This prestart task ensures a clean slate.
    task "cleanup" {
      driver    = "raw_exec"
      lifecycle {
        hook    = "prestart"
        sidecar = false
      }

      config {
        command = "/bin/sh"
        args    = ["-c", "pkill -9 -f penfold-worker 2>/dev/null; sleep 3; exit 0"]
      }

      resources {
        cpu    = 100
        memory = 64
      }
    }

    task "worker" {
      driver = "raw_exec"

      shutdown_delay = "5s"
      kill_signal    = "SIGTERM"
      kill_timeout   = "35s"

      config {
        # Use su instead of sudo — su on macOS properly forwards signals
        # to child processes, preventing zombie workers on restart.
        command = "/usr/bin/su"
        args    = ["-m", "james", "-c", "set -a; . /etc/penfold/worker.env; set +a; exec /opt/penfold/bin/penfold-worker"]
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
