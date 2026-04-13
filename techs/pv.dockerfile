FROM python:3.10

ARG gitlab_token_pv

# Install curl, git, git-lfs, and dos2unix
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    curl \
    git \
    git-lfs \
    dos2unix \
    p7zip-full && \
    rm -rf /var/lib/apt/lists/*


# Install pip using get-pip.py for Python 3.10
RUN curl -sS https://bootstrap.pypa.io/get-pip.py | python3.10


# Do not change the order of installation of these libraries!!!
RUN python3.10 -m pip install netCDF4==1.6.5
RUN python3.10 -m pip install NREL-PySAM==5.1.0
RUN python3.10 -m pip install pvlib==0.15.0
RUN python3.10 -m pip install numpy==1.26.4
RUN python3.10 -m pip install pandas==2.2.2
RUN python3.10 -m pip install python-dateutil
RUN python3.10 -m pip install python-dotenv
RUN python3.10 -m pip install xarray==2024.3.0
RUN python3.10 -m pip install flask==3.1.0
RUN python3.10 -m pip install gunicorn==23.0.0


# Clone PV simulation repo
WORKDIR /
ADD pysam-photovoltaic-simulation /pv_simulation

RUN 7z x /pv_simulation/merra_2_access/input_files.7z -o/pv_simulation/merra-2-db

RUN rm -rf /pv_simulation/merra_2_access/input_files.7z

# Set Environment Variables
RUN mv /pv_simulation/docker.env /pv_simulation/.env

# Set permissions
RUN chmod +x \
    /pv_simulation/scripts/start.sh \
    /pv_simulation/scripts/start.py

# Convert line endings to Unix format
RUN find /pv_simulation -type f -name "*.py" -exec dos2unix {} \;

EXPOSE 8082

ENTRYPOINT /pv_simulation/scripts/start.sh
WORKDIR "/pv_simulation"
