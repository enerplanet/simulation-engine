# Services

Each simulation service has one guide. Every guide follows the same shape, so
once you have read one you can read any of them quickly.

!!! info "What every service guide contains"
    1. **What it does** — the model and its output in one paragraph.
    2. **Prerequisites** — data or config the service needs before it will run.
    3. **Start & health-check** — bring it up and confirm it is ready.
    4. **Input specification** — the request format, field by field.
    5. **Consume it** — a copy-paste request and the expected result.
    6. **Output specification** — the response body and the files written.
    7. **Where output is stored** — exact paths on the shared volume.
    8. **Endpoints** — every route the service exposes.
    9. **Troubleshooting** — the failures you are most likely to hit.

## Available guides

| Service | Status | Guide |
| --- | --- | --- |
| BuEM — thermal building model | Ready | [Open](buem.md) |
| Ignis — heat demand | Planned | — |
| PV — photovoltaic yield | Planned | — |
| Calliope — energy-system optimisation | Planned | — |
| PyPSA — grid simulation | Planned | — |

Before reading a service guide, skim the **[Architecture](../architecture/overview.md)**
page once: it defines the endpoint pattern, the shared storage volume, and the
container names that every guide refers to.
