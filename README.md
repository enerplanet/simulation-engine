# EnerPlanET Simulation Engine

Dockerised backend that runs EnerPlanET's energy simulations — building
thermal load (BuEM, Ignis), photovoltaic yield (PV), and energy-system /
power-flow optimisation (Calliope, PyPSA) — behind one HTTP gateway.

Full operator manual: **[docs/](docs/index.md)** (built with MkDocs Material).
This README is just the quick start.

## Repository layout

```text
simulation-engine/
├── engine/                # Go gateway (webservice/) + Calliope/PyPSA runner (calliope-runner/)
├── environment/            # Docker setup, grouped by service
│   ├── docker-compose.*.yaml  # dev / unltd (single-instance) / unltd.dev (multi-instance, hot reload)
│   ├── haproxy/                # HAProxy configs for the multi-instance compose files
│   ├── building-model/         # ignis.dockerfile, buem.dockerfile
│   ├── renewables/             # pv.dockerfile + docker-compose.pv.yaml
│   └── gateway/                # calliope.dockerfile, calliope_gurobi.dockerfile, webservice.dockerfile
└── docs/                   # MkDocs source (architecture, per-service guides)
```

Each service Dockerfile clones its own public GitHub repo at build time — no
access tokens required. See [docs/architecture/overview.md](docs/architecture/overview.md)
for how a request flows through the gateway.

## Quick start

```bash
make build   # build the calliope-base and webservice images (first time only)
make up      # start the full stack
```

```bash
curl http://localhost:8089/health
```

| Command | Purpose |
| --- | --- |
| `make dev` | Hot-reload development stack |
| `make up-min` | Multi-instance stack (`docker-compose.unltd.dev.yaml`) |
| `make pv` | Start only the PV service |
| `make logs` | Follow container logs |
| `make down` / `make stop` | Stop the stack |
| `make clean` | Stop and remove containers + images |

Run `make help` for the full command list.

## Services

| Service | Models | Endpoint |
| --- | --- | --- |
| [BuEM](https://github.com/enerplanet/buem) | Hourly heating/cooling/electricity per building (ISO 52016-1 5R1C) | `POST /buem/start` |
| [Ignis](https://github.com/THD-Spatial-AI/ignis) | Annual heat demand per building (ISO 13790, TABULA) | `POST /ignis/start` |
| [PV](https://github.com/THD-Spatial-AI/pysam-photovoltaic-energy-simulation) | Photovoltaic yield time series | `POST /pv/start` |
| Calliope | Energy-system optimisation | `POST /calliope/start` |
| PyPSA | Power-system / grid simulation | via Calliope orchestration |

BuEM is a fork of [UU-BUEM/buem](https://github.com/UU-BUEM/buem) (MIT licence,
Utrecht University) — see [ATTRIBUTIONS.md](ATTRIBUTIONS.md).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) and the getting-started guides under
[docs/getting-started/](docs/getting-started/) (branch naming, commit
conventions, repository naming).
