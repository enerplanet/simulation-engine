FROM python:3.10

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
RUN python3.10 -m pip install numpy==1.26.4
RUN python3.10 -m pip install pandas==2.2.2
RUN python3.10 -m pip install python-dateutil==2.8.2
RUN python3.10 -m pip install python-dotenv
RUN python3.10 -m pip install xarray==2024.3.0
RUN python3.10 -m pip install flask==3.1.0
RUN python3.10 -m pip install gunicorn==23.0.0

# Clone wind simulation repo
WORKDIR /
ADD pysam-wind-energy-simulation /wind_simulation

RUN 7z x /wind_simulation/merra_2_access/input_files.7z -o/wind_simulation/merra-2-db
RUN cp /wind_simulation/merra_2_access/turbine_data.csv /wind_simulation/merra-2-db

RUN rm -rf /wind_simulation/merra_2_access/input_files.7z

# Set Environment Variables
RUN cp /wind_simulation/docker.env /wind_simulation/.env

# Set permissions
RUN chmod +x \
    /wind_simulation/scripts/start.sh \
    /wind_simulation/scripts/start.py

# Convert line endings to Unix format
RUN find /wind_simulation -type f -name "*.py" -exec dos2unix {} \;

EXPOSE 8083

ENTRYPOINT /wind_simulation/scripts/start.sh
WORKDIR "/wind_simulation"
