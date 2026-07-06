"""
Calliope2PyPSA:

Converts a Calliope Model to compute its Power Flow with PyPSA

"""

import logging
import sys
import argparse
from datetime import datetime
from pathlib import Path
from importlib.metadata import version

import c2p
import pypsa

c2p_version = version("c2p")

FORMAT = '[%(asctime)-15s] [ %(levelname)s ] [%(module)s]: %(message)s'
logging.basicConfig(format=FORMAT, level=logging.INFO) #filename='c2p.log'

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('path', type=str, help='Path to Calliope Model')

    args = parser.parse_args()
    path = Path(args.path)

    logging.info('Running C2P Version "%s"', c2p_version)
    logging.info('Running PyPSA Version "%s"', pypsa.__version__)
    logging.info('Start simulation for "%s"', path.name)

    a = c2p.c2p(path)

    # Create network, compute power flow
    logging.info('Computing network for "%s"', path.name)
    a.compute_net()
    
    # Save network as csv's and convert it for web/enerplanet
    logging.info('Saving network as CSV in the directory: %s', path)
    a.save_net(['csv'], web=True) 

    # Save power flow results as csv
    logging.info('Saving power flow results as CSV in the directory: %s', path)
    a.save_pf_results(['csv'])

    # Save settings as csv
    logging.info('Saving settings as CSV in the directory: %s', path)
    a.save_settings()

    logging.info('Finished simulations "%s"', path.name)

if __name__ == "__main__":
    main()
