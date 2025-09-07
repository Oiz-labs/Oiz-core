#!/bin/bash
set -e

OIZ_CONFIG=${OIZ_HOME}/config/config.toml
OIZ_GENESIS=${OIZ_HOME}/config/genesis.json

# Init genesis state if geth not exist
DATA_DIR=$(cat ${OIZ_CONFIG} | grep -A1 '\[Node\]' | grep -oP '\"\K.*?(?=\")')

GETH_DIR=${DATA_DIR}/geth
if [ ! -d "$GETH_DIR" ]; then
  geth --datadir ${DATA_DIR} init ${OIZ_GENESIS}
fi

exec "geth" "--config" ${OIZ_CONFIG} "$@"
