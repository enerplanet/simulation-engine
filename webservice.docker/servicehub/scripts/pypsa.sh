#!/bin/bash
#title           : pypsa.sh
#description     : starts c2p and pypsa
#author          : christina.sigl@th-deg.de
#date            : 2021-05-29
#version         : 0.7.0
#usage           : bash pypsa.sh <user_id> <model_id> <hex_id> <lkr> <callback_url>

model_id=$2
model_name='sim_'${model_id}
model_path=sims/${model_name}

c2p_main=c2p/main.py

logs=logs/pypsa.log

log_path=${model_path}/pypsa_output/
mkdir -p ${model_path}/pypsa_output

echo "======================================" >> ${logs}
echo "[$(date '+%Y-%m-%d %H:%M:%S')] PyPSA simulation starting" >> ${logs}
echo "  user_id: $1" >> ${logs}
echo "  model_id: $2" >> ${logs}
echo "  session_id: $3" >> ${logs}
echo "  lkr: $4" >> ${logs}
echo "  callback_url: $5" >> ${logs}
echo "  model_path: ${model_path}" >> ${logs}

#RUN C2P / PyPSA using system python (with PyPSA installed)
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Running C2P/PyPSA..." >> ${logs}
out=`python3.11 ${c2p_main} ${model_path} 2>&1`
exit_code=$?
echo "[$(date '+%Y-%m-%d %H:%M:%S')] C2P/PyPSA exit code: ${exit_code}" >> ${logs}

# Detect PF failures that are logged but do not raise a python process error.
if echo "${out}" | grep -q "network.pf() returned None after all retries"; then
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] Detected PF failure marker in C2P logs (forcing pypsa=false)" >> ${logs}
    exit_code=2
fi

# Additional defensive check: PF export skipped indicates missing PF result.
if echo "${out}" | grep -q "\"network.pf()\" is None, can not be exported"; then
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] Detected missing PF result marker (forcing pypsa=false)" >> ${logs}
    exit_code=2
fi

#SAVE LOG
echo "${out}" >> ${logs}
echo "${out}" | tee ${log_path}pypsa.log >> ${logs}
#SAVE EXIT CODE
echo ${exit_code} > ${log_path}exit_code.txt

sleep 1

echo "[$(date '+%Y-%m-%d %H:%M:%S')] Calling /pypsa/finish endpoint..." >> ${logs}
echo "  Will send file to callback_url: $5" >> ${logs}

if [ $exit_code -eq "0" ]; then
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] PyPSA succeeded, sending with pypsa=true" >> ${logs}
    finish_response=$(curl -s -w "\nHTTP_CODE:%{http_code}" -X POST -H "Content-Type: application/json" \
        --data '{"user_id": "'"$1"'", "model_id": "'"$2"'", "session_id": "'"$3"'", "lkr": "'"$4"'", "callback_url": "'"$5"'", "topology": [], "pypsa": true }' \
        --insecure http://localhost:8081/pypsa/finish 2>&1)
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] /pypsa/finish response: ${finish_response}" >> ${logs}
else
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] PyPSA failed (exit_code=${exit_code}), sending with pypsa=false" >> ${logs}
    finish_response=$(curl -s -w "\nHTTP_CODE:%{http_code}" -X POST -H "Content-Type: application/json" \
        --data '{"user_id": "'"$1"'", "model_id": "'"$2"'", "session_id": "'"$3"'", "lkr": "'"$4"'", "callback_url": "'"$5"'", "topology": [], "pypsa": false }' \
        --insecure http://localhost:8081/pypsa/finish 2>&1)
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] /pypsa/finish response: ${finish_response}" >> ${logs}
fi

echo "[$(date '+%Y-%m-%d %H:%M:%S')] PyPSA phase complete" >> ${logs}
echo "======================================" >> ${logs}
