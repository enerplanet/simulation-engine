"""
Creates DataFrames out of Calliope Results to apply PyPSA
"""

import logging
import re
from collections import Counter
from pathlib import Path

import numpy as np
import pandas as pd

log = logging.getLogger(__name__)

from .. import pattern as pt
from . import read

def get_topology(settings, reg_trafo):
    """
    Extracts data from config.json file

    Parameters
    ----------
    settings : 
            
    reg_trafo : 
    
    Returns
    ----------
    df_topo : pandas.DataFrame         

    """

    def _handle_pois(fo, to, trafo_suffix): 
        from_osm_id = str(fo['properties']['osm_id'])
        to_osm_id = str(to['properties']['osm_id'])

        from_id = str(fo['id'])
        to_id = str(to['id'])

        from_is_trafo = reg_trafo.search(from_osm_id)
        to_is_trafo = reg_trafo.search(to_osm_id)

        # Handle both sides independently - MV lines can connect two transformers
        if from_is_trafo:
            bus0 = from_osm_id + trafo_suffix
        else:
            bus0 = settings.prefix_poi + from_id
        
        if to_is_trafo:
            bus1 = to_osm_id + trafo_suffix
        else:
            bus1 = settings.prefix_poi + to_id
        
        return bus0, bus1

    def _check_trafo_usage(v_levels): #this will be done by EnerPlanET Configurator
        if 'mv' in v_levels and 'lv' in v_levels and not settings.trafo_mv_lv.used:
            log.warning('Trafo usage is "False" but LV and MV levels are defined ... set usage to "True"')
            settings.trafo_mv_lv.set_used(True)
        if 'hv' in v_levels and 'mv' in v_levels and not settings.trafo_hv_mv.used:
            log.warning('Trafo usage is "False" but MV and HV levels are defined ... set usage to "True"')
            settings.trafo_hv_mv.set_used(True)  

    config_json = read.json_file(settings.path.config_dir)

    key = settings.key
    
    def _positive_float(value, default, key):
        try:
            parsed = float(value)
        except (TypeError, ValueError):
            log.warning('Invalid value for %s=%s, using default %s', key, value, default)
            return float(default)
        if parsed <= 0:
            log.warning('Non-positive value for %s=%s, using default %s', key, value, default)
            return float(default)
        return parsed

    # PyPSA Settings
    pypsa_settings = config_json.get(key.pypsa, {})
    settings.trafo_mv_lv.set_type(pypsa_settings.get('trafo_mv_lv_type', settings.trafo_mv_lv.type))
    settings.line_mv.set_type(pypsa_settings.get('line_type_mv', settings.line_mv.type))
    settings.line_lv.set_type(pypsa_settings.get('line_type_lv', settings.line_lv.type))
    settings.trafo_mv_lv.num_parallel = _positive_float(
        pypsa_settings.get('trafo_mv_lv_num_parallel', settings.trafo_mv_lv.num_parallel),
        settings.trafo_mv_lv.num_parallel,
        'trafo_mv_lv_num_parallel',
    )
    settings.line_mv.num_parallel = _positive_float(
        pypsa_settings.get('line_mv_num_parallel', settings.line_mv.num_parallel),
        settings.line_mv.num_parallel,
        'line_mv_num_parallel',
    )
    settings.line_lv.num_parallel = _positive_float(
        pypsa_settings.get('line_lv_num_parallel', settings.line_lv.num_parallel),
        settings.line_lv.num_parallel,
        'line_lv_num_parallel',
    )
    settings.trafo_mv_lv.set_used(pypsa_settings.get('trafo_mv_lv_used', False))
    settings.trafo_hv_mv.set_used(False)
    
    # Topology Settings
    topo_data = config_json[key.topology]

    topo_rows = []

    for i in topo_data:
        a = i[key.fr]
        b = i[key.to]

        pipe = i[key.pipe]
        llength = i[key.length]

        if pipe == 'lv':
            bus0, bus1 = _handle_pois(a, b, settings.suffix_trafo_bus_lv)
            ltype = settings.line_lv.type
            num_parallel = settings.line_lv.num_parallel
        elif pipe == 'mv':
            bus0, bus1 = _handle_pois(a, b, settings.suffix_trafo_bus_mv)
            ltype = settings.line_mv.type
            num_parallel = settings.line_mv.num_parallel
        elif pipe == 'hv':
            bus0, bus1 = _handle_pois(a, b, settings.suffix_trafo_bus_hv)
            if hasattr(settings, 'line_hv'):
                ltype = settings.line_hv.type
                num_parallel = settings.line_hv.num_parallel
            else:
                raise ValueError('HV pipe requested but line_hv is not configured in settings')
        else:
            raise ValueError('Invalid value %s for pipe, expects lv, mv or hv' % pipe)

        lname = bus0 + '_' + bus1
        topo_rows.append({'bus0': bus0, 'bus1': bus1, 'length': llength, 'pipe': pipe, 'type': ltype, 'num_parallel': num_parallel, '_name': lname})

    df_topo = pd.DataFrame(topo_rows)
    df_topo.index = df_topo.pop('_name')
    settings.vlevels = df_topo[key.pipe].unique()
    _check_trafo_usage(settings.vlevels)

    return df_topo

class base:
    """
    Base class for topology and tech(nology)
    """

    def __init__(self, settings):
        self.settings = settings
        self.prefix_bus = settings.prefix_bus
    
    def _add_prefix(self, df, col=None):
        if not df.empty:
            if col is None:
                df.index = self.prefix_bus + df.index.astype(str)
            else:
                df[col] = self.prefix_bus + df[col].astype(str)

class topology(base):
    """ 
    Uses topology data from config.json file to create net topology (buses, trafos, lines) 
    """

    def __init__(self, settings): 
        super().__init__(settings)

        self.reg_trafo = re.compile(pt.trafo_pattern)
        self.topo_data = get_topology(settings, self.reg_trafo)

        self.trafo_mv_lv = self.settings.trafo_mv_lv
        self.trafo_hv_mv = self.settings.trafo_hv_mv

        self.line_lv = self.settings.line_lv
        self.line_mv = self.settings.line_mv
        #self.line_hv = self.settings.line_hv

        self.v_nom = self.settings.v_nom

    def _append_bus(self, df, bus, v_nom):
        return pd.concat([df, pd.Series({'v_nom': v_nom}, name=bus).to_frame().T])   
    
    def _append_line(self, df, bus0, bus1, ltype, length, name, num_parallel=1.0):
        return pd.concat([df, pd.Series({'bus0': bus0, 'bus1': bus1, 'type': ltype, 'length': length, 'num_parallel': num_parallel}, name=name).to_frame().T])

    def _append_trafo(self, df, bus0, bus1, ttype, name, num_parallel):
        return pd.concat([df, pd.Series({'bus0': bus0, 'bus1': bus1, 'type': ttype, 'num_parallel': num_parallel}, name=name).to_frame().T])

    def _get_v_nom(self, v_level):
        v_level = v_level.lower()
        if 'lv' in v_level:
            return self.v_nom.lv
        elif 'mv' in v_level:
            return self.v_nom.mv
        elif 'hv' in v_level:
            return self.v_nom.hv
        else:
            log.error('Can not set bus nominal voltage')

    def _get_buses(self, topology):
        return np.unique(pd.concat([topology['bus0'], topology['bus1']]))

    def bus_trafo(self):
        """
        Creates input dataframes for bus and trafo to use in pypsa-method network.import_components_from_dataframe(df, "<component_name>")
        
        Returns
        ----------
        df_bus : pandas.DataFrame
            df with buses and corresponding nominal voltage
        df_trafo : pandas.DataFrame
           df with trafos
        """     
        self.v_nom.update()
        topo_data = self.topo_data
        topo_data = topo_data.drop(columns=['num_parallel'])

        self.df_trafo = pd.DataFrame()
        self.df_bus = pd.DataFrame() 

        trafo_buses = []
        
        #unique names required
        buses = self._get_buses(topo_data)

        for bus in buses:
            v_level = np.unique(np.append(topo_data.loc[topo_data['bus0'] == bus, 'pipe'].values, topo_data.loc[topo_data['bus1'] == bus, 'pipe'].values))
            if not v_level.size == 1:
                log.error('No unique voltage level')
            v_nom = self._get_v_nom(str(v_level))

            self.df_bus = self._append_bus(self.df_bus, bus, v_nom) 

            if self.reg_trafo.search(str(bus)):
                trafo_buses.append(bus)

        if self.trafo_mv_lv.used:
            trafo_buses = np.unique(trafo_buses)
            trafo_list = np.unique([trafo[:-3] for trafo in trafo_buses])
        
            for trafo in trafo_list:
                trafo_name = trafo
                buses = [b for b in trafo_buses if trafo_name.lower() in b.lower()]
                if len(buses) == 1:
                    bus1 = buses[0]
                    if bus1[-len(self.settings.suffix_trafo_bus_lv):] == self.settings.suffix_trafo_bus_lv:
                        v_nom = self.v_nom.mv
                        bus0 = trafo_name + self.settings.suffix_trafo_bus_mv
                    else:
                        v_nom = self.v_nom.lv
                        bus0 = trafo_name + self.settings.suffix_trafo_bus_lv
                    #add virtual point for trafo at another voltage level
                    self.df_bus = self._append_bus(self.df_bus, bus0, v_nom)
                elif len(buses) == 2:
                    # Ensure bus0 has higher voltage than bus1 (PyPSA convention)
                    b0, b1 = buses[0], buses[1]
                    v0 = self.df_bus.loc[b0, 'v_nom'] if b0 in self.df_bus.index else 0
                    v1 = self.df_bus.loc[b1, 'v_nom'] if b1 in self.df_bus.index else 0
                    if v0 >= v1:
                        bus0, bus1 = b0, b1
                    else:
                        bus0, bus1 = b1, b0
                        log.info('Swapped transformer buses to ensure bus0 (%s) has higher voltage', bus0)
                else: 
                    raise ValueError('Error not implemented')

                self.df_trafo = self._append_trafo(self.df_trafo, bus0, bus1, self.trafo_mv_lv.type, trafo_name, self.trafo_mv_lv.num_parallel)

        self._add_prefix(self.df_bus)
        self._add_prefix(self.df_trafo, 'bus0')
        self._add_prefix(self.df_trafo, 'bus1')

        return self.df_bus, self.df_trafo
    
    def line(self):
        """ 
        Creates input dataframe for lines to use in pypsa-method network.import_components_from_dataframe(df, "Line")

        Returns
        ----------
        df_line : pandas.DataFrame
            
        """     

        self.df_line = self.topo_data
        self.df_line = self.df_line.drop(columns=['pipe'])
        
        self._add_prefix(self.df_line, 'bus0')
        self._add_prefix(self.df_line, 'bus1')

        return self.df_line


def get_unique_techs(data, tech_name, idx, snaps, key):
    """ 
    Gives nonunique techs an unique name and arranges data for use in pypsa-method network.madd (Unique names as PyPSA input required)

    Parameters
    ----------
    tech_name : str
        name of tech to arrange
    idx : pandas.Int64Index
        indexes corresponding to tech
    
    Returns
    ----------
    p_sets : ndarray
        n-dim array with corresponding data
    techs : list
        list of unique names for tech
    buses : list
        list of corresponding locations/buses
    """

    i=0
    buses = [] 
    techs = []
    p_sets = []   

    while i < len(idx):
        techs.append(tech_name)
        buses.append(str(data['locs'][idx[i]]))
        p_sets = np.append(p_sets, [[data[key][idx[i]:idx[i]+snaps]]])
        i=i+snaps    

    p_sets = p_sets.reshape(len(buses),snaps)

    return p_sets, techs, buses

def check_snaps(idx, snaps):
    """ 
    Checks if timesteps for tech fits general snaps
    
    Parameters
    ----------
    idx : list
    snaps : int

    Returns
    ----------
    : bool    
    """    

    if len(idx) == snaps:
        return True
    elif not (len(idx)/snaps).is_integer():
        raise ValueError('Num of timesteps does not match snaps')
    else: 
        return False

def check_path(path):
    return path.exists()

def make_gen_df(df, bus, name, control):
    if isinstance(bus, list):
        return pd.concat([df, pd.DataFrame({'bus': bus, 'control': [control]*len(name)}, index=name)])
    else:
        return pd.concat([df, pd.Series({'bus': bus,'control': control}, name=name).to_frame().T])

def make_load_df(df, bus, name):
    if isinstance(bus, list):
        return pd.concat([df, pd.DataFrame({'bus': bus}, index=name)])
    else:
        return pd.concat([df, pd.Series({'bus': bus}, name=name).to_frame().T])

def make_df(df, bus, name, control=None):
    if control:
        return make_gen_df(df, bus, name, control)
    else:
        return make_load_df(df, bus, name)
        
def arrange_duplicates(df_tech, df_pset):
    idx_list = df_tech.index.tolist()
    for (k,v) in Counter(df_tech.index).items():
        if v > 1:
            idxs = [i for i, x in enumerate(df_tech.index) if x == k]
            a=0
            for _idx in idxs:
                a=a+1
                idx_list[_idx] = k + '_' + str(a)  
                
    df_tech.index = idx_list
    df_pset.columns = idx_list
    
    return df_tech, df_pset

def check_input_format(settings, csv): 
    if check_path(settings.path.nc_dir) and not csv:
        return False
    elif check_path(settings.path.prod_dir) or check_path(settings.path.con_dir):
        return True
    else:
        raise FileNotFoundError('Can not find calliope model files, neither netcdf nor csv')

class tech(base):
    def __init__(self, settings):
        super().__init__(settings)
        self.input_csv = check_input_format(settings, settings.csv_techs)
        self.data = read.data(settings, self.input_csv)
                
    def _set_attr(self):
        """ 
        Sets pattern and suffixes/prefixes for either generator or load
    
        Parameters
        ----------
        gen : bool
        settings : from class settings

        Returns
        ----------
        pattern_tech : _sre.SRE_Pattern
        pattern_slack : _sre.SRE_Pattern or NoneType
        prefix_tech : str
        suffix_tech : str
        key : str
        df : DataFrame
        """ 

        if self.gen:
            self.pattern_tech = re.compile(pt.gen_tech_pattern)
            self.pattern_tech_mv = re.compile(pt.gen_tech_mv_pattern)
            self.pattern_slack = re.compile(pt.gen_slack_pattern)
            self.prefix_tech = self.settings.prefix_gen
            self.suffix_tech = self.settings.suffix_gen
            self.key = self.settings.key.gen
            self.df = self.data.carrier_prod()
        else:
            self.pattern_tech = re.compile(pt.load_tech_pattern)
            self.pattern_tech_mv = re.compile(pt.load_tech_mv_pattern)
            self.pattern_slack = None
            self.prefix_tech = self.settings.prefix_load
            self.suffix_tech = self.settings.suffix_load 
            self.key = self.settings.key.load
            self.df = self.data.carrier_con()
    
    def _tech_df(self, df, tech, bus, df_pset, pset):    
        if self.pattern_tech.search(tech):
            tech_name = self.prefix_tech + tech + '_' + bus + self.suffix_tech           
            df_pset[tech_name] = pset            
            if self.gen:
                if self.pattern_slack.search(tech): 
                    if self.settings.trafo_mv_lv.used and self.settings.trafo_mv_lv.slack:
                        # attach grid supply to trafo bus at the higher voltag level end
                        bus = bus + self.settings.trafo_mv_lv.bus0_suffix
                    else:
                        if 'lv' in self.settings.vlevels:
                            bus = bus + self.settings.trafo_mv_lv.bus1_suffix
                        elif 'mv' in self.settings.vlevels:
                            bus = bus + self.settings.trafo_mv_lv.bus0_suffix
                        else:
                            log.error('Error setting slack bus')
                    return make_df(df, bus, tech_name, control='Slack'), df_pset   
                else: 
                    return make_df(df, bus, tech_name, control='PQ'), df_pset  
            else:   
                return make_df(df, bus, tech_name), df_pset
        else:
            return df, df_pset

class technology(tech):
    def __init__(self, settings):
        super().__init__(settings)

    def _netcdf(self, df_tech, df_pset):
        for tech in self.df.index:
            pset = self.df.loc[tech].values
            bus = re.search('(.+?)::', tech).group(1)
            tech_name = re.search('::(.+?):', tech).group(1)
            df_tech, df_pset = self._tech_df(df_tech, tech_name, bus, df_pset, pset) 

        return df_tech, df_pset
    
    def _csv(self, df_tech, df_pset):
        
        def arrange_p_set(i):
            """ 
            Converts (active) power values to fit the required unit for papsa input.

            Parameters
            ----------
            i : numpy.int64
                value for index

            Returns
            ----------
            p_set : ndarray
                n-dim array with corresponding data
            """
            p_set = np.array(self.df[self.key][i:i+self.snaps]) 
            return p_set

        _, self.snaps = self.data.times()

        techs = list(dict.fromkeys(self.df['techs']))
        for tech in techs:
            tech_idx = self.df.index[self.df['techs'] == tech]
            if not check_snaps(tech_idx, self.snaps):
                p_sets, names, buses = get_unique_techs(self.df, tech, tech_idx, self.snaps, self.key)
                for i in range(len(buses)):
                    df_tech, df_pset = self._tech_df(df_tech, names[i], buses[i], df_pset, p_sets[i])
            else:
                pset = arrange_p_set(tech_idx[0])
                bus = self.df['locs'][tech_idx[0]]
                df_tech, df_pset = self._tech_df(df_tech, tech, bus, df_pset, pset)

        return df_tech, df_pset

    def _create(self):
        self._set_attr()

        df_pset = pd.DataFrame()
        df_tech = pd.DataFrame()
        
        if self.input_csv:
            df_tech, df_pset = self._csv(df_tech, df_pset)
        else:
            df_tech, df_pset = self._netcdf(df_tech, df_pset)
                 
        if not df_tech.index.is_unique:
            df_tech, df_pset = arrange_duplicates(df_tech, df_pset)
        
        df_pset = df_pset.reindex(sorted(df_pset.columns), axis=1)
        self._add_prefix(df_tech, 'bus')

        return df_tech.sort_index(), df_pset.abs()

    def generator(self):
        self.gen = True
        
        return self._create()

    def load(self):
        self.gen = False

        return self._create()

    def times(self):

        return self.data.times()

class data:
    def __init__(self, settings):
        self.tech = technology(settings)
        self.topo = topology(settings)
    def bus(self):
        log.info('Bus and Trafo dataframes')
        return self.topo.bus_trafo()
    def line(self):
        log.info('Line dataframes')
        return self.topo.line()
    def generator(self):
        log.info('Generator dataframes')
        return self.tech.generator()
    def load(self):
        log.info('Load dataframes')
        return self.tech.load()
    def snaps(self):
        _ , snaps = self.tech.times()
        return snaps 
    def timesteps(self):
        timesteps, _ = self.tech.times()
        return timesteps
