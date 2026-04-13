#!/bin/bash

LOG_FILE="logs/calliope.log"

echo "======================================" >> ${LOG_FILE}
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Calliope simulation starting" >> ${LOG_FILE}
echo "  user_id: $1" >> ${LOG_FILE}
echo "  model_id: $2" >> ${LOG_FILE}
echo "  session_id: $3" >> ${LOG_FILE}
echo "  lkr: $4" >> ${LOG_FILE}
echo "  callback_url: $5" >> ${LOG_FILE}
echo "  pypsa_enabled: $6" >> ${LOG_FILE}

echo "[$(date '+%Y-%m-%d %H:%M:%S')] Running calliope..." >> ${LOG_FILE}
# Use calliope from dedicated virtualenv with compatible pandas/numpy versions
/opt/calliope-venv/bin/calliope run ./sims/sim_$2/model.yaml --save_csv=./sims/sim_$2/results >> ${LOG_FILE} 2>&1
calliope_exit=$?
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Calliope exit code: ${calliope_exit}" >> ${LOG_FILE}

# Copy tech resource CSV files into separate subdirectories per tech type
# so they are included in the result ZIP sent to the frontend.
tech_resources_dir="./sims/sim_$2/tech_resources"
tech_count=0
for tech in pv wind biomass geothermal; do
    for f in ./data/$2/${tech}_*.csv; do
        if [ -e "$f" ]; then
            mkdir -p "${tech_resources_dir}/${tech}"
            cp -L "$f" "${tech_resources_dir}/${tech}/"
            tech_count=$((tech_count + 1))
        fi
    done
done
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Copied ${tech_count} tech resource files to ${tech_resources_dir}" >> ${LOG_FILE}

# Clean up model-specific data directory (PV/wind/biomass output + symlinks)
# Calliope has finished reading these files; results are in sims/sim_$2/results
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Cleaning up model data directory: ./data/$2" >> ${LOG_FILE}
rm -rf ./data/$2

echo "[$(date '+%Y-%m-%d %H:%M:%S')] Calling /calliope/finish endpoint..." >> ${LOG_FILE}
finish_response=$(curl -s -w "\nHTTP_CODE:%{http_code}" -X POST -H "Content-Type: application/json" \
    --data '{"user_id": "'"$1"'", "model_id": "'"$2"'", "session_id": "'"$3"'", "lkr": "'"$4"'", "callback_url": "'"$5"'", "topology": [], "pypsa": '"$6"', "calliope_exit": '"${calliope_exit}"' }' \
    --insecure http://localhost:8081/calliope/finish 2>&1)
echo "[$(date '+%Y-%m-%d %H:%M:%S')] /calliope/finish response: ${finish_response}" >> ${LOG_FILE}

# Only run PyPSA if calliope succeeded AND pypsa is enabled
if [ ${calliope_exit} -ne 0 ]; then
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] Calliope failed (exit code ${calliope_exit}), skipping PyPSA" >> ${LOG_FILE}
elif [ "$6" = "true" ]; then
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] Calling /pypsa/start endpoint..." >> ${LOG_FILE}
    pypsa_response=$(curl -s -w "\nHTTP_CODE:%{http_code}" -X POST -H "Content-Type: application/json" \
        --data '{"user_id": "'"$1"'", "model_id": "'"$2"'", "session_id": "'"$3"'", "lkr": "'"$4"'", "callback_url": "'"$5"'", "topology": [], "pypsa": true }' \
        --insecure http://localhost:8081/pypsa/start 2>&1)
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] /pypsa/start response: ${pypsa_response}" >> ${LOG_FILE}
else
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] PyPSA simulation skipped (disabled in request)" >> ${LOG_FILE}
fi

echo "[$(date '+%Y-%m-%d %H:%M:%S')] Calliope phase complete" >> ${LOG_FILE}
echo "======================================" >> ${LOG_FILE}
