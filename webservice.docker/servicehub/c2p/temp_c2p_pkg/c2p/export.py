"""
Exports c2p results.
"""

import logging
import re

import numpy as np
import pandas as pd

from .settings import create_var_df


log = logging.getLogger(__name__)


def path_check(path):
    if not path.exists():
        path.mkdir()


def create_new_csv_file_name(file, suffix):
    return file[:-4] + suffix + file[-4:]


def get_files(path):
    return [f.name for f in path.iterdir() if (path / f.name).is_file()]


def rad_to_grad(i):
    return i * 180 / np.pi


class export:
    def __init__(self, settings):
        self.settings = settings

    def net(self, net, formats=['csv', 'hdf5', 'netcdf'], path=None, name=None):
        """
        Exports pypsa network to csv, hdf5, netcdf.

        Parameters
        ----------
        net : pypsa.components.Network
            pypsa network to export
        formats : list
            list with formats to export to (csv, hdf5, netcdf)
            e.g. formats=['csv', 'hdf5', 'netcdf']
        path : str
            path to store output file/folder
            (default) use path from settings
        name : str
            (model) name for output file/folder
            (default) use name from settings
        """

        er = []
        if path is None:
            path = self.settings.path.output_dir
        if name is None:
            name = self.settings.path.model_name

        path_check(path)

        csv_name = 'csv_' + name
        h5_name = name + '.h5'
        nc_name = name + '.nc'

        for f in formats:
            if 'csv' in f.lower():
                log.info('Network to csv')
                try:
                    net.export_to_csv_folder(path / csv_name)
                except Exception as e:
                    log.error(e)
            elif 'hdf5' in f.lower():
                log.info('Network to hdf5')
                try:
                    net.export_to_hdf5(path / h5_name)
                except Exception as e:
                    log.error(e)
            elif 'netcdf' in f.lower():
                log.info('Network to netcdf')
                try:
                    net.export_to_netcdf(path / nc_name)
                except Exception as e:
                    log.error(e)
            else:
                er.append(f)
        if er:
            raise ValueError('Expects csv, hdf5, netcdf but got:', er)

    def web_csv(self, timesteps):
        """
        Adds timesteps, changes unit for reading in EnerPlanET.

        Parameters
        ----------
        timesteps : list
            list with timesteps for each data row
        """

        self.timesteps = timesteps
        csv_folder_path = self.settings.path.output_csv_dir
        unit = 1 / self.settings.params.unit
        folder_name = self.settings.path.output_csv_web_folder
        new_folder = csv_folder_path / folder_name

        csv_files = get_files(csv_folder_path)
        # TODO why varying index names?
        key_index1 = 'Unnamed: 0'
        key_index2 = 'snapshots'
        key_timesteps = 'timesteps'
        v_ang = re.compile(r'buses-v_ang', re.IGNORECASE)
        v_mag = re.compile(r'buses-v_mag', re.IGNORECASE)

        path_check(new_folder)
        i = 0
        csv_files.sort()
        for csv in csv_files:
            file_name = create_new_csv_file_name(csv, '-t')
            df = pd.read_csv(csv_folder_path / csv, engine='python')
            key = None
            if key_index1 in df.keys():
                key = key_index1
            elif key_index2 in df.keys():
                key = key_index2

            if key is not None:
                i=i+1
                log.info('[%s] Convert %s for web to %s', i, csv, file_name)
                df = df.drop(columns=key)
                # Add timesteps for usage in EnerPlanET
                # Use pd.concat instead of insert to avoid PerformanceWarning for fragmented frames
                timesteps_df = pd.DataFrame({key_timesteps: self.timesteps})
                # Ensure indices match for concatenation
                timesteps_df.index = df.index
                df = pd.concat([timesteps_df, df], axis=1)

                if v_ang.search(csv):
                    # Save voltage angles also in grad.
                    df.to_csv(new_folder / file_name, index=False)
                    df.iloc[:, 1::] = rad_to_grad(df.iloc[:, 1::])
                    file_name = create_new_csv_file_name(csv, '-t-grad')
                    i=i+1
                    log.info('[%s] Convert %s for web to %s', i, csv, file_name)
                elif v_mag.search(csv):
                    # Ignores voltage magnitude.
                    pass
                elif 'snapshots' in csv:
                     # Ignore snapshots file for unit conversion
                    pass
                else:
                    # Change unit back to fit calliope settings for comparison
                    # in EnerPlanET.
                    # Convert to float to avoid LossySetitemError/TypeError with int columns
                    df.iloc[:, 1::] = df.iloc[:, 1::].astype(float) * unit

                # Save files as csv (required by EnerPlanET).
                df.to_csv(new_folder / file_name, index=False)
    def view(self):
        return create_var_df(vars(self))


def build_df(full_pf, timesteps=pd.Series()):
    # Handle potentially multi-column results (multiple sub-networks)
    n_iter_vals = full_pf['n_iter']
    if len(n_iter_vals.shape) > 1:
        n_iter_vals = n_iter_vals.max(axis=1)
    
    converged_vals = full_pf['converged']
    if len(converged_vals.shape) > 1:
        converged_vals = converged_vals.all(axis=1)
    
    error_vals = full_pf['error']
    if len(error_vals.shape) > 1:
        error_vals = error_vals.max(axis=1)
    
    # Use index from full_pf as default snapshots
    snapshots = n_iter_vals.index
    
    # If timesteps are provided, try to use them if lengths match
    if not timesteps.empty:
        if len(timesteps) == len(n_iter_vals):
            snapshots = timesteps.values
        else:
            log.warning('Timesteps length (%d) mismatch with results length (%d). Using results index instead.', 
                        len(timesteps), len(n_iter_vals))
    
    df = pd.DataFrame({
        'snapshots': snapshots,
        'n_iter': n_iter_vals.values,
        'converged': converged_vals.values,
        'error': error_vals.values
    }, index=n_iter_vals.index)
    return df


def df(df, path, name, formats=['csv', 'hdf5', 'excel'], index=True):
    """
    Saves pandas.DataFrame to csv, hdf5, excel

    Parameters
    ----------
    df : pandas.DataFrame
        df to save
    text : str
        text for logging
    formats : list
        list with formats to export to (csv, hdf5, excel)
        e.g. formats=['csv', 'hdf5', 'excel']
    """

    er = []
    csv_name = name + '.csv'
    h5_name = name + '.h5'
    excel_name = name + '.xlsx'

    path_check(path)

    for f in formats:
        if 'csv' in f.lower():
            log.info('Save "' + name + '" to csv')
            try:
                df.to_csv(path / csv_name, index=index)
            except Exception as e:
                log.error(e)
        elif 'hdf5' in f.lower():
            log.info('Save "' + name + '" to hdf5')
            try:
                df.to_hdf(path / h5_name, key=name, index=index)
            except Exception as e:
                log.error(e)
        elif 'excel' in f.lower():
            log.info('Save "' + name + '" to excel')
            try:
                df.to_excel(path / excel_name, index=index)
            except Exception as e:
                log.error(e)
        else:
            er.append(f)
    if er:
        raise ValueError('Expects csv, hdf5, excel but got:', er)