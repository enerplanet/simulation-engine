# Use Python base image (has ca-certificates pre-installed for HTTPS)
FROM python:3.11-bookworm

# Set noninteractive mode for apt-get to suppress prompts
ENV DEBIAN_FRONTEND=noninteractive

# Update and install required packages
RUN apt-get update && apt-get upgrade -y && \
    apt-get install -y coinor-cbc coinor-libcbc-dev make golang-go git curl && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# set temp directory
WORKDIR /tmp

# Pin numpy<2.0 for pyomo compatibility, pandas<2.0 for Calliope 0.6.10 compatibility
RUN pip install --upgrade pip setuptools wheel && \
    pip install "numpy>=1.24.0,<2.0.0" "pandas>=1.5.0,<2.0.0" && \
    pip install calliope==0.6.10

# Set the default working directory for the container
WORKDIR "/"

