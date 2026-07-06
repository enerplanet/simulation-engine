###### README ->
#
# 0) (Only once/the first time) Download all required files for building the image
# * cd engine/webservice/
# * Put your data into the directory engine/webservice/data
#
# 1)
# git rev-parse HEAD > webservice/git-commit-sha
#
# 2) Create image (takes some time)
# cd into engine
# sudo docker build --tag s6et_webservice -f webservice.dockerfile . --platform linux/amd64
#
# 3) Create container from recently generated image run it...
# @ tcf-dl1:
# sudo docker run -it \
#                  --restart=always \
#                  --name S6ET_Webservice \
#                  -p 192.168.80.100:8081:8081 \
#                  s6et_webservice:latest
#
#
### For debug/dev purpose only:
# sudo docker build --tag webservicebsp -f webservice.dockerfile .
# sudo docker create -it --name MyWebservicebspBash webservicebsp:latest bash
# sudo docker start -a -i MyWebservicebspBash
###### <- README


# Set the base image on which you want to build up your image/application
FROM s6et_calliope:latest

# Webservice
ADD calliope-runner /webservice/
#WORKDIR /tmp

ADD webservice /tmp/webservice
WORKDIR /tmp/webservice
RUN make
WORKDIR /tmp/webservice/build
RUN cp webservice /webservice/webservice

RUN chmod +x /webservice/start.sh
RUN chmod +x /webservice/webservice

# Create a separate virtualenv for Calliope with pinned compatible versions
# This prevents PyPSA dependencies from breaking Calliope
RUN python -m venv /opt/calliope-venv && \
    /opt/calliope-venv/bin/pip install --upgrade pip && \
    /opt/calliope-venv/bin/pip install "numpy>=1.23.0,<1.24.0" "pandas>=1.5.0,<2.0.0" "xarray>=2022.3.0,<2023.0.0" calliope==0.6.10 highspy

# Patch Pyomo's appsi LegacySolver for Calliope 0.6.10 + highspy 1.13 compatibility:
# - Accept/ignore solver_io kwarg from SolverFactory
# - Add warmstart + **kwargs to solve() 
# - Expose self.name attribute for Calliope's solver name check
RUN python3.11 -c "\
path='/opt/calliope-venv/lib/python3.11/site-packages/pyomo/contrib/appsi/base.py'; \
f=open(path,'r'); c=f.read(); f.close(); \
c=c.replace( \
'            class LegacySolver(LegacySolverInterface, cls):\n                pass', \
'            class LegacySolver(LegacySolverInterface, cls):\n                _solver_name = name\n                def __init__(self, **kwds):\n                    kwds.pop(\'solver_io\', None)\n                    super().__init__(**kwds)\n                    self.name = self._solver_name\n                pass'); \
c=c.replace( \
'              keepfiles: bool = False,\n              symbolic_solver_labels: bool = False):', \
'              keepfiles: bool = False,\n              symbolic_solver_labels: bool = False,\n              warmstart: bool = False,\n              **kwargs):'); \
f=open(path,'w'); f.write(c); f.close(); print('Pyomo patched')"

# Setup PyPSA (c2p) with latest versions in system python
RUN pip install --upgrade pip
# Upgrade plotly first (calliope pins old version 3.10.0 which lacks graph_objects)
RUN pip install --upgrade "plotly>=5.0.0"
# Install c2p 1.0.5 (fix unit conversion, transformer fallback, locals() bug, vectorized time series)
RUN pip install /webservice/c2p/c2p-1.0.5-py3-none-any.whl \
    "pandas>=2.0.0" \
    "numpy>=1.24.0,<2.0.0" \
    "xarray>=2023.1.0" \
    "pypsa>=1.0.0" \
    "linopy>=0.3.0" \
    highspy \
    netcdf4 \
    openpyxl

# Replace polars with CPU-compatible build (avoids SIGILL on servers without AVX2)
RUN pip uninstall -y polars polars-runtime-32 && \
    pip install polars-lts-cpu

# Define the port on which the webservice listens
EXPOSE 8081

# Start the webservice
ENTRYPOINT /webservice/start.sh

# Set workdir
WORKDIR "/webservice"
