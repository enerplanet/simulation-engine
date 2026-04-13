# Start with the Gurobi optimizer image as the base image - using Python 3.9
FROM gurobi/python:12.0.1_3.9

# update system and certificates
RUN apt-get update \
    && apt-get install --no-install-recommends -y \
       ca-certificates \
       curl \ 
       git \
       make \
       golang-go \
       software-properties-common \
    && update-ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Install setuptools
RUN python3.9 -m pip install --upgrade pip setuptools

# set temp directory
WORKDIR /tmp

# Install Calliope
RUN python3.9 -m pip install calliope==0.6.10

# Set Python 3.9 as default
RUN update-alternatives --install /usr/bin/python python /usr/bin/python3.9 1 && \
    update-alternatives --install /usr/bin/python3 python3 /usr/bin/python3.9 1

# Ensure Gurobi executables are in the PATH
ENV PATH="/opt/gurobi/linux64/bin:${PATH}"
ENV LD_LIBRARY_PATH="/opt/gurobi/linux64/lib:${LD_LIBRARY_PATH}"

# Copy the Gurobi license file (this will be mounted via Docker Compose)
COPY gurobi.lic /opt/gurobi/gurobi.lic

# Set the working directory to root
WORKDIR "/"