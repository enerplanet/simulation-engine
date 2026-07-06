"""
Calliope to PyPSA
"""

import logging

import pandas as pd

from . import export as ex
from .inputs.create import data
from .net import create_network_df
from .settings import settings


log = logging.getLogger(__name__)
      

class c2p:
    def __init__(self, path_to_calliope_model):
        self.settings = settings(path_to_calliope_model)
        self.path = self.settings.path
        self.params = self.settings.params
        self.export = ex.export(self.settings)
        self.model_name = self.settings.model_name

        self.df_bus = pd.DataFrame()
        self.df_trafo = pd.DataFrame()
        self.df_line = pd.DataFrame()
        self.df_gen = pd.DataFrame()
        self.df_load = pd.DataFrame()

        self.net = None
        self.full_pf = None


    def _get_data(self):     
        return data(self.settings)

    def _remove_invalid_pf_exports(self):
        raw_files = {
            'buses-p.csv',
            'buses-q.csv',
            'buses-v_ang.csv',
            'buses-v_mag_pu.csv',
            'generators-p.csv',
            'generators-q.csv',
            'lines-p0.csv',
            'lines-p1.csv',
            'lines-q0.csv',
            'lines-q1.csv',
            'transformers-p0.csv',
            'transformers-p1.csv',
            'transformers-q0.csv',
            'transformers-q1.csv',
        }
        web_files = {
            'buses-p-t.csv',
            'buses-q-t.csv',
            'buses-v_ang-t.csv',
            'buses-v_ang-t-grad.csv',
            'buses-v_mag_pu-t.csv',
            'generators-p-t.csv',
            'generators-q-t.csv',
            'lines-p0-t.csv',
            'lines-p1-t.csv',
            'lines-q0-t.csv',
            'lines-q1-t.csv',
            'transformers-p0-t.csv',
            'transformers-p1-t.csv',
            'transformers-q0-t.csv',
            'transformers-q1-t.csv',
        }

        for directory, file_names in (
            (self.path.output_csv_dir, raw_files),
            (self.path.output_csv_dir / self.path.output_csv_web_folder, web_files),
        ):
            for file_name in file_names:
                file_path = directory / file_name
                if not file_path.exists():
                    continue
                try:
                    file_path.unlink()
                    log.warning('Removed invalid PF export %s', file_path)
                except Exception as e:
                    log.warning('Failed to remove invalid PF export %s: %s', file_path, e)
        
    def make_net(self):
        """
        Generates PyPSA network and computes its power flows.
        """
        self.get = self._get_data()
   
        # CREATE BUSES AND TRAFOS    
        self.df_bus, self.df_trafo  = self.get.bus()
 
        # CREATE LINES
        self.df_line = self.get.line()    

        # CREATE GENERATORS
        self.df_gen, gen_pset = self.get.generator() 

        # CREATE LOADS
        self.df_load, load_pset = self.get.load()

        # CREATE PYPSA-NETWORK
        self.net = create_network_df(self.model_name, self.get.snaps(), 
                                     self.df_bus, self.df_trafo, 
                                     self.df_line, df_gen=self.df_gen, 
                                     gen_pset=gen_pset*self.params.unit, 
                                     df_load=self.df_load, 
                                     load_pset=load_pset*self.params.unit) 
        return self.net
        
    def compute_net(self):
        self.make_net()      
        #compute net
        if self.settings.con_check:
            log.info('Consistency check')
            self.net.consistency_check()
        self.net.lpf()
        seed=False
        try:
            if self.settings.trafo_mv_lv.used:
                seed=True          
            # Distribute slack must be False so we don't incorrectly distribute line losses to PV generators
            self.full_pf = self.net.pf(use_seed=seed, distribute_slack=False)
            
            # Reject PF-derived exports if any snapshot failed to converge.
            if self.full_pf is not None and 'converged' in self.full_pf:
                converged_count = int(self.full_pf['converged'].values.sum())
                total_count = int(self.full_pf['converged'].size)
                if converged_count < total_count:
                    log.error('Power flow failed to converge for some or all snapshots. Clearing bad results.')
                    self.full_pf = None
                    
        except Exception as e:
            log.error('network.pf() is %s', e)

    def _check_net(self):
        if not self.net is None: 
            return self.net   
        else: 
            log.info('No network available, compute network with make_net()')

    def get_net(self):
        """
        Returns PyPSA network
        """
        return self._check_net()

    def save_net(self, formats=['csv', 'hdf5', 'netcdf'], web=True):
        """ 
        Saves PyPSA network.

        Parameters
        ----------
        formats : list
            list with formats to export network
        web : bool
            (default) True, exports network as csv and converts csv for web
        """

        if self._check_net():
            if web:
                csv = [True for x in formats if 'csv' in x.lower()]
                if not csv:
                    formats.append('csv')
                self.export.net(self.net, formats)
                if self.full_pf is not None:
                    self.export.web_csv(self.get.timesteps())
                else:
                    self._remove_invalid_pf_exports()
            else:        
                self.export.net(self.net, self.path.output_dir, 
                                self.model_name, formats)
                     
    def save_pf_results(self, formats=['csv', 'hdf5', 'excel']):
        """ 
        Saves Power Flow results (network.pf()).

        Parameters
        ----------
        formats : list
            list with formats to export network.pf()-results
        """

        if self._check_net():
            if self.full_pf is not None:
                res_df = ex.build_df(self.full_pf, self.get.timesteps()) 
                ex.df(res_df, self.path.output_dir, self.path.pf_result_name, 
                      formats, True)
            else:
                log.warning('"network.pf()" is None, can not be exported')

    def save_settings(self):
        """
        Saves all settings.
        """ 

        self.settings.save()

    def save(self):
        """
        Saves all available data.
        """
        
        self.save_net()
        self.save_pf_results()
        self.save_settings()
