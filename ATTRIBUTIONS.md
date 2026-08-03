# Attributions

The simulation engine bundles and runs third-party open-source software. This
file credits those sources and records the licence obligations we carry.

## BuEM — Building Energy Model

The thermal building-model service is **BuEM**, developed by a researcher at
**Utrecht University**. We do not modify the science; we run a fork of the
project pinned to a branch that fixes packaging for our container build.

| | |
| --- | --- |
| Upstream project | <https://github.com/UU-BUEM/buem> |
| Author / copyright | Somadutta Sahoo |
| Institution | Utrecht University — CETP programme, funded by NWO (Dutch Research Council) |
| Licence | MIT |
| Documentation | <https://buem.readthedocs.io> |
| Fork we deploy | `https://github.com/enerplanet/buem` (branch `enerplanet`) |
| Built by | `environment/building-model/buem.dockerfile` |

BuEM's building typology is itself derived from the
[TABULA/EPISCOPE](https://episcope.eu/) project (IEE TABULA — Typology Approach
for Building Stock Energy Assessment).

!!! note "MIT licence obligation"
    The MIT licence requires the original copyright notice and permission notice
    to be retained. The fork keeps the upstream `LICENSE` file
    (© 2025 Somadutta Sahoo) inside the container, satisfying this obligation.
    Do not strip it.
