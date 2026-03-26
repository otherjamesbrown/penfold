# Penfold Backend

## CoBuild

This project uses [CoBuild](https://github.com/otherjamesbrown/cobuild) for pipeline automation — designs flow through structured phases (design → decompose → implement → review → done) with quality gates.

**Read `.cobuild/AGENTS.md` for full pipeline instructions, commands, and task completion protocol.**

## Building

```bash
make build              # Build all modules
make test               # Test all modules
make vet                # go vet all modules
```

## Deploying

```bash
penf deploy gateway     # Build + deploy gateway to dev02 (systemd)
penf deploy worker      # Build + deploy worker to dev01 (launchd)
penf deploy ai          # Build + deploy AI coordinator to dev02 (systemd)
penf deploy all         # Deploy all in dependency order
penf deploy --status    # Check all services
```

## Quick Reference

| System | Server | Config |
|--------|--------|--------|
| Penfold | dev02.brown.chat:50051 | ~/.penf/config.yaml |
| Context Palace | dev02.brown.chat:5432 | ~/.cobuild/config.yaml |

```bash
penf status / penf health / penf update
cobuild wi list --type task
./scripts/deploy.sh status
```
