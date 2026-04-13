"""
General Settings
"""

import logging
import re
from pathlib import Path

import pandas as pd

log = logging.getLogger(__name__)

from . import export
from . import pattern as pt

dir_name = Path(__file__).parent

default_types_file = dir_name / 'defaults/types.csv'
default_affixes_file = dir_name / 'defaults/affixes.csv'
default_voltages_file = dir_name / 'defaults/voltages.csv'
default_unit_file = dir_name / 'defaults/unit.csv'
paths_file = dir_name / 'defaults/paths.csv'
trafo_types_file = dir_name / 'defaults/trafos.csv'
line_types_file = dir_name / 'defaults/lines.csv'

def get_id(model_name):
    resu = re.match(pt.hexagon_pattern, model_name)
    if resu is None:
        raise ValueError('Can not extract model_id from model_name, the following pattern is required: {pattern}'.format(pattern=pt.hexagon_pattern))
    else:
        return resu.group(1) 

def check_path(path):
    if path.exists():
        return path
    else:
        raise NotADirectoryError(" '%s' is not a directory" % (path)) 

def create_var_df(Dict, used=False):
    reg = re.compile(r'(.)>$')  
    df = pd.DataFrame()
    df['names'] = Dict.keys()
    df['values'] = Dict.values()
    if used:
        df = df.dropna()
        for idx in range(len(df)):
            if reg.search(str(df['values'][idx])):
                df = df.drop([idx])
            elif not str(df['values'][idx]).strip():
                print('leer' , idx)
                df = df.drop([idx])
        return df
    else:
        return df

def set_v_nom(trafo_type):
    """ 
    Sets nominal voltage for a trafo type

    Parameters
    ----------
    trafo_type : str
        type of transformer (from pypsa/pandapower standard types)
    Returns
    ----------
    v_nom_0 : float
    v_nom_1 : float
    """
    v_nom_0 = re.match(r'(.*) MVA (.*)/(.*) kV', trafo_type).group(2)
    v_nom_1 = re.match(r'(.*) MVA (.*)/(.*) kV', trafo_type).group(3)
    
    return v_nom_0, v_nom_1

def read_defaults(df, pattern):
    pat = re.compile(pattern)
    for idx in df.index:
        if pat.search(df['name'][idx]): 
            return df['value'][idx]
    else:
        raise ValueError('default value not defined')

class line:
    def __init__(self, ltype):
        self.type = ltype
        self.num_parallel = 1.0

    def available_types(self):
        return pd.read_csv(line_types_file)

    def view(self):
        return(create_var_df(vars(self)))

class line_lv(line):
    def __init__(self, ltype):
        super().__init__(ltype)
        self.ltype = ltype

    def available_types(self):
        df = super().available_types()
        return df[df['name'].str.contains("LV")].reset_index(drop=True)

    def set_type(self, typ):
        if typ in self.available_types()['value'].values:
            self.type = typ
        else:
            self.type = self.ltype
            log.error('[Invalid] LV line type "%s", use default value "%s" instead', typ, self.type)

class line_mv(line):
    def __init__(self, ltype):
        super().__init__(ltype)
        self.ltype = ltype

    def available_types(self):
        df = super().available_types()
        return df[df['name'].str.contains("MV")].reset_index(drop=True)

    def set_type(self, typ):
        if typ in self.available_types()['value'].values:
            self.type = typ
        else:
            self.type = self.ltype
            log.error('[Invalid] MV line type "%s", use default value "%s" instead', typ, self.type)

class line_hv(line):
    def __init__(self, ltype):
        super().__init__(ltype)
        self.ltype = ltype

    def available_types(self):
        df = super().available_types()
        return df[df['name'].str.contains("HV")].reset_index(drop=True)

    def set_type(self, typ):
        if typ in self.available_types()['value'].values:
            self.type = typ
        else:
            self.type = self.ltype
            log.error('[Invalid] HV line type "%s", use default value "%s" instead', typ, self.type)


class trafo:
    def __init__(self, ttype, bus0, bus1):
        """ 
        Transformer Base Class
    
        Parameters
        ----------
        ttype : str
            type of the transformer, has to fit pypsa/pandapower transformer types

        used : bool
            default False: transformer is not used
        slack : bool
            default False: slack bus is the bus connected at the lower voltage level, if true slack bus is connected at the higher voltage end
        """
        self.type = ttype
        self.used = False
        self.bus0_suffix = bus0 
        self.bus1_suffix = bus1 
        self.slack = True
        self.num_parallel = 1.0
        self.v_nom0, self.v_nom1 = self.get_v_noms() 

    def update(self):
        self.v_nom0, self.v_nom1 = self.get_v_noms()   

    def get_v_noms(self):
        return set_v_nom(self.type) 

    def set_used(self, u=True):
        log.info('Set trafo used to %s (%s)', u, self.type)
        self.used = u

    def view(self):
        return(create_var_df(vars(self)))

    def view_used(self):
        return(create_var_df(vars(self), True))
        
    def available_types(self):
        return pd.read_csv(trafo_types_file)    

class trafo_mv_lv(trafo):
    def __init__(self, ttype, bus0, bus1):
        super().__init__(ttype, bus0, bus1)
        self.ttype = ttype

    def available_types(self):
        df = super().available_types()
        return df[df['name'].str.contains("MV_LV")]
    
    def set_type(self, typ):
        if typ in self.available_types()['value'].values:
            self.type = typ
        else:
            self.type = self.ttype
            log.error('[Invalid] MV-LV trafo type "%s", use default value "%s" instead', typ, self.type)

class trafo_hv_mv(trafo):
    def __init__(self, ttype, bus0, bus1):
        super().__init__(ttype, bus0, bus1)
        self.ttype = ttype
        
    def available_types(self):
        df = super().available_types()
        return df[df['name'].str.contains("HV_MV")]

    def set_type(self, typ):
        if typ in self.available_types()['value'].values:
            self.type = typ
        else:
            self.type = self.ttype
            log.error('[Invalid] HV-MV trafo type "%s", use default value "%s" instead', typ, self.type)

class v_nom:
    def __init__(self, tml, thm):
        #use default_voltages
        self.tml = tml
        self.thm = thm

        df_default_voltages = pd.read_csv(default_voltages_file)
        for n in df_default_voltages.index:
            self.__dict__.update({df_default_voltages.values[n][0]: df_default_voltages.values[n][1]})

    def update(self):
        self.thm.update()
        self.tml.update()

        if self.thm.used and self.tml.used:
            if self.thm.v_nom1 == self.tml.v_nom0:
                self.lv = self.tml.v_nom1
                self.mv = self.thm.v_nom1
                self.hv = self.thm.v_nom0
            else: 
                raise ValueError('Incompatible Trafos %s  and %s, mismatch of voltage levels', self.thm.type, self.tml.type)
        elif self.thm.used and not self.tml.used:
            raise ValueError('Only use of HV-MV trafo is not supported')
        elif not self.thm.used and self.tml.used: 
            self.lv = self.tml.v_nom1
            self.mv = self.tml.v_nom0
        else:
            #use default values
            log.info('Default voltages are used')      
        
    def view(self):
        return(create_var_df(vars(self)))
    
    def view_used(self):
        return(create_var_df(vars(self), True))

class default_types:
    def __init__(self):
        #create global default values
        df_default_types = pd.read_csv(default_types_file)
        df_default_unit = pd.read_csv(default_unit_file)

        for n in df_default_types.index:
            self.__dict__.update({df_default_types.values[n][0]: df_default_types.values[n][1]})
            
        for n in df_default_unit.index:
            self.__dict__.update({df_default_unit.values[n][0]: df_default_unit.values[n][1]})

    def view_used(self):
        return create_var_df(vars(self), True)
    
    def view(self):
        return create_var_df(vars(self))

class key:
    def __init__(self):
        #create global default values
        self.gen = 'carrier_prod'
        self.load = 'carrier_con'
        self.carrier = 'carriers'
        self.bus = 'locs'
        self.tech = 'techs'
        self.timestep = 'timesteps'
        self.bus0 = 'from'
        self.bus1 = 'to'
        self.len = 'length'

        #config.json
        self.pypsa = 'pypsa'
        self.topology = 'topology'
        self.fr = 'from'
        self.to = 'to'
        self.length = 'length'
        self.pipe = 'pipe'
    

    def view_used(self):
        return create_var_df(vars(self), True)
    
    def view(self):
        return create_var_df(vars(self))

class base:
   def __init__(self, path_to_calliope_model):
        self.model_dir = check_path(Path(path_to_calliope_model))
        self.model_name = self.model_dir.name
        self.model_id = get_id(self.model_name)  

        self.csv_techs = False         
        self.vlevels = []   

        df_default_affixes = pd.read_csv(default_affixes_file)
        df_default_affixes['value'] = df_default_affixes['value'].fillna('')
        
        #for n in df_default_affixes.index:
        #    self.__dict__.update({df_default_affixes.values[n][0]: df_default_affixes.values[n][1]})
        self.prefix_poi = read_defaults(df_default_affixes, pt.prefix_poi_pattern)      
        self.prefix_bus = read_defaults(df_default_affixes, pt.prefix_bus_pattern)
        self.prefix_gen = read_defaults(df_default_affixes, pt.prefix_gen_pattern)
        self.suffix_gen = read_defaults(df_default_affixes, pt.suffix_gen_pattern)
        self.prefix_load = read_defaults(df_default_affixes, pt.prefix_load_pattern)
        self.suffix_load = read_defaults(df_default_affixes, pt.suffix_load_pattern)
        self.prefix_bus_id = read_defaults(df_default_affixes, pt.prefix_bus_id_pattern)

        self.suffix_trafo_bus_lv = read_defaults(df_default_affixes, pt.suffix_trafo_bus_lv_pattern)
        self.suffix_trafo_bus_mv = read_defaults(df_default_affixes, pt.suffix_trafo_bus_mv_pattern)
        self.suffix_trafo_bus_hv = read_defaults(df_default_affixes, pt.suffix_trafo_bus_hv_pattern)

class paths:
    def __init__(self, model_name, model_dir):
        self.model_dir = model_dir
        self.model_name = model_name
        
        df_paths = pd.read_csv(paths_file)
        for n in df_paths.index:
            self.__dict__.update({df_paths.iloc[n, 0]: df_paths.iloc[n, 1]})
            
        def check_for_model_id(name):
            a = re.match(pt.model_id_pattern, name)
            if a is not None:
                new_name = a.group(1) + self.model_id + a.group(2)
                return new_name  
        
        def check_for_model_name(name):
            a = re.match(pt.model_name_pattern, name)
            if a is not None:
                new_name = a.group(1) + self.model_name + a.group(2)
                return new_name 

        def check_for_variables(name):
            a = check_for_model_id(name)
            if a:
                return a
            else:
                a = check_for_model_name(name)
                if a:
                    return a
      
        for i in range(len(df_paths)):
            a = check_for_variables(df_paths.iloc[i, 1])
            if a:
                setattr(self, df_paths.iloc[i, 0], a)
        

        _output_folder = self.output_folder
        _import_csv_dir = self.model_dir / self.import_csv_folder

        #exports
        self.output_dir = self.model_dir / _output_folder
        self.output_csv_dir = self.output_dir / self.output_csv_folder
        #imports
        self.config_dir = self.model_dir / self.config_file      
        self.prod_dir = _import_csv_dir / self.csv_prod_file
        self.con_dir = _import_csv_dir / self.csv_con_file
        self.nc_dir = self.model_dir / self.nc_file

    #only for test cases, set output folder name in defaults/path_names.csv  
    def set_output_folder(self, folder_name):
        self.output_dir = self.model_dir / folder_name
        self.output_csv_dir = self.output_dir / self.output_csv_folder
       
    def view(self):
        return create_var_df(vars(self))

class settings(base):
    def __init__(self, path_to_calliope_model):
        super().__init__(path_to_calliope_model)
        self.params = default_types()
        self.key = key()
        self.path = paths(self.model_name, self.model_dir)

        self.line_lv = line_lv(self.params.line_type_lv)
        self.line_mv = line_mv(self.params.line_type_mv)
        if hasattr(self.params, 'line_type_hv') and self.params.line_type_hv:
            self.line_hv = line_hv(self.params.line_type_hv)

        self.trafo_mv_lv = trafo_mv_lv(self.params.trafo_type_mv_lv, self.suffix_trafo_bus_mv, self.suffix_trafo_bus_lv)
        self.trafo_hv_mv = trafo_hv_mv(self.params.trafo_type_hv_mv, self.suffix_trafo_bus_hv, self.suffix_trafo_bus_mv)
        self.v_nom = v_nom(self.trafo_mv_lv, self.trafo_hv_mv)

        self.con_check = True

    def view_used(self):
        return create_var_df(vars(self), True)
    
    def view(self):
        return create_var_df(vars(self))

    def save(self, formats=['csv']):
        def row (df, name, var):
            return pd.concat([df, pd.DataFrame([{'name': name, 'value': var}])], ignore_index=True)
        
        #TODO descide which values to save
        df = pd.DataFrame()
        df = row(df, 'model_name', self.path.model_name)
        df = row(df, 'unit', self.params.unit)
        df = row(df, 'line_type_lv', self.line_lv.type)
        df = row(df, 'line_type_mv', self.line_mv.type)
        df = row(df, 'trafo_mv_lv_used', self.trafo_mv_lv.used)
        if self.trafo_mv_lv.used:
            df = row(df, 'trafo_type_mv_lv', self.trafo_mv_lv.type)
            df = row(df, 'mv_slack', self.trafo_mv_lv.slack)
            df = row(df, 'v_nom_mv', self.v_nom.mv)
        df = row(df, 'v_nom_lv', self.v_nom.lv)
        
        export.df(df, self.path.output_dir, self.path.settings_name, formats, False)