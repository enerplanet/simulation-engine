FROM continuumio/miniconda3

RUN apt-get update \
    && apt-get install -y --no-install-recommends curl coinor-cbc \
    && rm -rf /var/lib/apt/lists/*

# BuEM is developed at Utrecht University (author: Somadutta Sahoo, MIT licence).
# Upstream: https://github.com/UU-BUEM/buem  —  see ATTRIBUTIONS.md.
# We build a fork that only fixes container packaging and API; the model is unchanged.
# Public repo — no access token required.
WORKDIR /
RUN git clone --branch enerplanet https://github.com/enerplanet/buem.git

WORKDIR /buem
RUN conda env create -f environment_docker.yml && conda clean -afy

ENV PATH=/opt/conda/envs/buem_env/bin:$PATH
ENV PYTHONPATH=/buem/src

WORKDIR /buem/src

EXPOSE 5000

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD curl -fsS http://localhost:5000/api/health || exit 1

CMD gunicorn --bind 0.0.0.0:5000 "buem.apis.api_server:create_app()" --workers "${GUNICORN_WORKERS:-2}" --threads 1
