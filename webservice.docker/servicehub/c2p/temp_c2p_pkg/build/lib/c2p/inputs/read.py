"""
Reads calliope output data (csv or netcdf).
"""

import json
import logging
import re

import pandas as pd
import xarray as xr

from .. import pattern as pt

log = logging.getLogger(__name__)


class extract:
    """ 
    Extracts relevant data from dataframe according to pattern.
        
    Parameters
    ----------
    pattern : str
        regular expression
    key : str
        key to extract from csv column (from_key)
        (default) None
    """

    def __init__(self, pattern=None, key=None):
        self.pattern = pattern
        self.key = key 

        if not self.pattern is None: 
            self.reg = re.compile(self.pattern)  

    def _check_pattern(self):
        if self.pattern is None: 
            log.warning('No pattern is applied to extract data')
            return False
        else:
            return True

    def from_key(self, df):
        """
        Calliope csv file: Keep only elements defined in pattern, drop 
        all others.
        """

        if self._check_pattern():
            if not df.empty:
                if self.key in df.keys():
                    for b in df[self.key]:
                        if not self.reg.search(b): 
                            df = df.drop(df.index[df[self.key] == b])
                    df.reset_index(drop=True)
                    if df.empty:
                        log.warning('DataFrame is empty, no entries in \
                                    key "%s" match pattern "%s"',
                                    self.key, extract.pattern)
                    return df
                else:
                    log.warning('Key "%s" does not exist', self.key)
            return pd.DataFrame()
        return df

    def from_index(self, df):
        """
        Calliope netcdf file: keep only elements defined in pattern, 
        drop all others.
        """

        if self._check_pattern():
            for b in df.index:
                if not self.reg.search(b): 
                    df = df.drop(b)
        return df

def json_file(path):
    """
    Reads a json file.
    """

    try:
        with open(path) as f:
            data = json.load(f)
        return data
    except Exception as e:
        raise e
        
def csv_file(path, name):
    """
    Reads calliope csv files.

    Parameters
        ----------
        path : Path
            
        name : str
            (only for logging)
    """
    try:
        data = pd.read_csv(path) 
        return data
    except Exception as e:
        log.error(e)
        log.warning('Values for %s are not defined', name)
        #return pd.DataFrame() 
        raise e

def get_timings(*dfs):
    """ 
    Extracts time varing data from calliope csv file.

    Parameters
    ----------
    *dfs : DataFrame(s)
        path to 'results_carrier_prod.csv'
    carrier_con_path : Path
        path to 'results_carrier_con.csv'
    extract : extract
        applies pattern to read data

    Returns
    ----------
    timesteps : pd.Series

    snaps : int
        number of snapshots
    """

    key = 'timesteps'

    #defaults
    timesteps = None 
    snaps = 1

    for df in dfs: 
        if not df.empty:
            if key in df.keys():
                timesteps = df[key].drop_duplicates()
                timesteps = pd.to_datetime(timesteps, errors='coerce')  # Convert to datetime64 with precision
                snaps = len(timesteps)
                return timesteps, snaps
            else:
                #log.warning('Dataframe has no key timesteps')
                pass
            
    log.warning('Failed to get timesteps and snaps from data, return \
                default values')
    return timesteps, snaps 

class csv:
    """ 
    Reads calliope csv results (only carrier_prod, carrier_con files).

    Parameters
    ----------
    settings : c2p.settings
        for path to 'results_carrier_prod.csv' and 'results_carrier_con.csv'
    extract : extract
        applies pattern to read data
    """

    def __init__(self, settings, extract):
        self.carrier_prod_path = settings.path.prod_dir
        self.carrier_con_path = settings.path.con_dir 
        self.extract = extract   

    def all(self):
        return self.carrier_prod(), self.carrier_con()

    def carrier_prod(self):
        carrier_prod = self.extract.from_key(csv_file(self.carrier_prod_path, 
                                             'generators'))
        if not carrier_prod.empty:
            self.timesteps, self.snaps = get_timings(carrier_prod)
        return carrier_prod

    def carrier_con(self):
        carrier_con = self.extract.from_key(csv_file(self.carrier_con_path, 
                                            'loads'))
        if not carrier_con.empty:
            self.timesteps, self.snaps = get_timings(carrier_con)
        return carrier_con

    def times(self):
        if hasattr(self, 'timesteps') and hasattr(self, 'snaps'):
            return self.timesteps, self.snaps
        else:
            return get_timings(self.carrier_prod(), self.carrier_con())

class netcdf:
    """ 
    Reads calliope netcdf file.

    Parameters
    ----------
    settings : c2p.settings
        for path to 'result.nc'
    extract : extract
        applies pattern to read data
    """

    def __init__(self, settings, extract):    
        self.nc_ds = xr.open_dataset(settings.path.nc_dir)
        self.extract = extract

    def all(self):
        return self.carrier_prod(), self.carrier_con()

    def carrier_prod(self):
        return self.extract.from_index(self.nc_ds['carrier_prod'].to_pandas())

    def carrier_con(self):
        return self.extract.from_index(self.nc_ds['carrier_con'].to_pandas())

    def times(self):
        snaps = self.nc_ds['timesteps'].size
        timesteps = pd.Series(self.nc_ds['timesteps'], name='timesteps') 
        return timesteps, snaps

def data(settings, input_csv):
    extra = extract(pt.carrier_pattern, settings.key.carrier)
    if input_csv:
        log.info('CSV files')
        return csv(settings, extra)    
    else:
        log.info('NetCDF file')
        return netcdf(settings, extra)
