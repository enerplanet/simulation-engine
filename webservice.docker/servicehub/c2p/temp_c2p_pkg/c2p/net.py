"""
Construct PyPSA Network
Updated for PyPSA 1.0+ API (madd -> add with DataFrame)
"""

import logging

import pandas as pd
import pypsa

log = logging.getLogger(__name__)

DEFAULT_LINE_TYPE_LV = 'NAYY 4x150 SE'
DEFAULT_LINE_TYPE_MV = 'NA2XS2Y 1x185 RM/25 12/20 kV'

# Custom transformer types for 20/0.4 kV that are not in PyPSA standard library
# Parameters based on standard distribution transformer specifications
# vsc = short-circuit voltage (%), vscr = resistive part (%), pfe = iron losses (kW), i0 = no-load current (%)
# Naming convention matches PyLovo: Tr_250, Tr_400, Tr_630 (kVA sizes)
CUSTOM_TRANSFORMER_TYPES = {
    # PyLovo standard sizes (250, 400, 630 kVA)
    'Tr_250': {
        'f_nom': 50.0, 's_nom': 0.25, 'v_nom_0': 20.0, 'v_nom_1': 0.4,
        'vsc': 4.0, 'vscr': 1.0, 'pfe': 0.6, 'i0': 0.4,
        'phase_shift': 150.0, 'tap_side': 0, 'tap_neutral': 0, 'tap_min': -2, 'tap_max': 2, 'tap_step': 2.5
    },
    'Tr_400': {
        'f_nom': 50.0, 's_nom': 0.4, 'v_nom_0': 20.0, 'v_nom_1': 0.4,
        'vsc': 4.0, 'vscr': 1.05, 'pfe': 0.93, 'i0': 0.35,
        'phase_shift': 150.0, 'tap_side': 0, 'tap_neutral': 0, 'tap_min': -2, 'tap_max': 2, 'tap_step': 2.5
    },
    'Tr_630': {
        'f_nom': 50.0, 's_nom': 0.63, 'v_nom_0': 20.0, 'v_nom_1': 0.4,
        'vsc': 4.0, 'vscr': 1.06, 'pfe': 1.3, 'i0': 0.32,
        'phase_shift': 150.0, 'tap_side': 0, 'tap_neutral': 0, 'tap_min': -2, 'tap_max': 2, 'tap_step': 2.5
    },
    # Legacy names for backward compatibility
    '0.25 MVA 20/0.4 kV': {
        'f_nom': 50.0, 's_nom': 0.25, 'v_nom_0': 20.0, 'v_nom_1': 0.4,
        'vsc': 4.0, 'vscr': 1.0, 'pfe': 0.6, 'i0': 0.4,
        'phase_shift': 150.0, 'tap_side': 0, 'tap_neutral': 0, 'tap_min': -2, 'tap_max': 2, 'tap_step': 2.5
    },
    '0.4 MVA 20/0.4 kV': {
        'f_nom': 50.0, 's_nom': 0.4, 'v_nom_0': 20.0, 'v_nom_1': 0.4,
        'vsc': 4.0, 'vscr': 1.05, 'pfe': 0.93, 'i0': 0.35,
        'phase_shift': 150.0, 'tap_side': 0, 'tap_neutral': 0, 'tap_min': -2, 'tap_max': 2, 'tap_step': 2.5
    },
    '0.63 MVA 20/0.4 kV': {
        'f_nom': 50.0, 's_nom': 0.63, 'v_nom_0': 20.0, 'v_nom_1': 0.4,
        'vsc': 4.0, 'vscr': 1.06, 'pfe': 1.3, 'i0': 0.32,
        'phase_shift': 150.0, 'tap_side': 0, 'tap_neutral': 0, 'tap_min': -2, 'tap_max': 2, 'tap_step': 2.5
    },
    # Extended sizes for larger installations
    '0.1 MVA 20/0.4 kV': {
        'f_nom': 50.0, 's_nom': 0.1, 'v_nom_0': 20.0, 'v_nom_1': 0.4,
        'vsc': 4.0, 'vscr': 1.2, 'pfe': 0.35, 'i0': 0.5,
        'phase_shift': 150.0, 'tap_side': 0, 'tap_neutral': 0, 'tap_min': -2, 'tap_max': 2, 'tap_step': 2.5
    },
    '0.16 MVA 20/0.4 kV': {
        'f_nom': 50.0, 's_nom': 0.16, 'v_nom_0': 20.0, 'v_nom_1': 0.4,
        'vsc': 4.0, 'vscr': 1.3, 'pfe': 0.46, 'i0': 0.45,
        'phase_shift': 150.0, 'tap_side': 0, 'tap_neutral': 0, 'tap_min': -2, 'tap_max': 2, 'tap_step': 2.5
    },
    '0.8 MVA 20/0.4 kV': {
        'f_nom': 50.0, 's_nom': 0.8, 'v_nom_0': 20.0, 'v_nom_1': 0.4,
        'vsc': 6.0, 'vscr': 1.325, 'pfe': 1.9, 'i0': 0.3,
        'phase_shift': 150.0, 'tap_side': 0, 'tap_neutral': 0, 'tap_min': -2, 'tap_max': 2, 'tap_step': 2.5
    },
    '1 MVA 20/0.4 kV': {
        'f_nom': 50.0, 's_nom': 1.0, 'v_nom_0': 20.0, 'v_nom_1': 0.4,
        'vsc': 6.0, 'vscr': 1.2, 'pfe': 2.3, 'i0': 0.28,
        'phase_shift': 150.0, 'tap_side': 0, 'tap_neutral': 0, 'tap_min': -2, 'tap_max': 2, 'tap_step': 2.5
    },
    '1.25 MVA 20/0.4 kV': {
        'f_nom': 50.0, 's_nom': 1.25, 'v_nom_0': 20.0, 'v_nom_1': 0.4,
        'vsc': 6.0, 'vscr': 1.15, 'pfe': 2.8, 'i0': 0.26,
        'phase_shift': 150.0, 'tap_side': 0, 'tap_neutral': 0, 'tap_min': -2, 'tap_max': 2, 'tap_step': 2.5
    },
    '1.6 MVA 20/0.4 kV': {
        'f_nom': 50.0, 's_nom': 1.6, 'v_nom_0': 20.0, 'v_nom_1': 0.4,
        'vsc': 6.0, 'vscr': 1.1, 'pfe': 3.5, 'i0': 0.24,
        'phase_shift': 150.0, 'tap_side': 0, 'tap_neutral': 0, 'tap_min': -2, 'tap_max': 2, 'tap_step': 2.5
    },
    '2 MVA 20/0.4 kV': {
        'f_nom': 50.0, 's_nom': 2.0, 'v_nom_0': 20.0, 'v_nom_1': 0.4,
        'vsc': 6.0, 'vscr': 1.0, 'pfe': 4.2, 'i0': 0.22,
        'phase_shift': 150.0, 'tap_side': 0, 'tap_neutral': 0, 'tap_min': -2, 'tap_max': 2, 'tap_step': 2.5
    },
    '2.5 MVA 20/0.4 kV': {
        'f_nom': 50.0, 's_nom': 2.5, 'v_nom_0': 20.0, 'v_nom_1': 0.4,
        'vsc': 6.0, 'vscr': 0.95, 'pfe': 5.0, 'i0': 0.20,
        'phase_shift': 150.0, 'tap_side': 0, 'tap_neutral': 0, 'tap_min': -2, 'tap_max': 2, 'tap_step': 2.5
    },
}


def add_custom_transformer_types(network):
    """Add custom transformer types to the network if not already present."""
    for ttype, params in CUSTOM_TRANSFORMER_TYPES.items():
        if ttype not in network.transformer_types.index:
            network.transformer_types.loc[ttype] = params
            log.info('Added custom transformer type: %s', ttype)


def create_network_df(model_id, timesteps, df_bus, df_trafo, df_line, df_gen=pd.DataFrame(), gen_pset=pd.DataFrame(), df_load=pd.DataFrame(), load_pset=pd.DataFrame()):      
    """ 
    Creates a PyPSA network with buses, trafos, lines, generators, loads

    Parameters
    ----------
    model_id : str
        name of model
    timesteps : int
        length of snapshots/timeseries
    df_bus : pandas.DataFrame
        
    df_trafo : pandas.DataFrame

    df_line : pandas.DataFrame

    df_gen : pandas.DataFrame
    
    gen_pset : pandas.DataFrame 

    df_load : pandas.DataFrame

    load_pset : pandas.DataFrame
    
    Returns
    ----------
    network : pypsa.Network
        pypsa network component
    """   
    # Create network container
    network = pypsa.Network(name=model_id)
    log.info('Create PyPSA network')
    
    # Add custom transformer types that are not in PyPSA standard library
    add_custom_transformer_types(network)
    
    # Set time varying data first (required before adding components with time series)
    network.set_snapshots(range(0, timesteps, 1))
    
    #------------------------------------------------------------------------------------------------------    
    # Add buses (PyPSA 1.0+ supports DataFrame-based add())
    if not df_bus.empty:
        # Prepare DataFrame with required columns
        buses_df = pd.DataFrame(index=df_bus.index)
        buses_df['v_nom'] = df_bus['v_nom']
        network.add("Bus", buses_df.index, v_nom=buses_df['v_nom'])
        log.info('Added %d Buses', len(buses_df))
    #------------------------------------------------------------------------------------------------------    
    # Add trafos (vectorized)
    if not df_trafo.empty:
        num_parallel = df_trafo['num_parallel'] if 'num_parallel' in df_trafo.columns else 1
        
        # Validate transformer types - check all are in library (including custom types)
        valid_trafo_types = set(network.transformer_types.index)
        trafo_types = df_trafo['type'].copy()
        for idx, ttype in trafo_types.items():
            if ttype not in valid_trafo_types:
                log.error('Transformer type "%s" not found in standard or custom types. '
                          'Available types: %s', ttype, list(valid_trafo_types)[:10])
                raise ValueError(f'Unknown transformer type: "{ttype}". '
                                 f'Register it in CUSTOM_TRANSFORMER_TYPES or use a standard type.')
        
        network.add(
            "Transformer",
            df_trafo.index,
            bus0=df_trafo['bus0'],
            bus1=df_trafo['bus1'],
            type=trafo_types,
            num_parallel=num_parallel
        )
        log.info('Added %d Trafos', len(df_trafo))
    else:
        log.info('Not added: PyPSA network has no trafos')
    #------------------------------------------------------------------------------------------------------
    # Add lines (vectorized)
    if not df_line.empty:
        line_df = df_line.copy()

        # Drop self-loop lines early.
        self_loop_mask = line_df['bus0'] == line_df['bus1']
        if self_loop_mask.any():
            self_loop_count = int(self_loop_mask.sum())
            log.warning('Dropping %d self-loop lines (bus0 == bus1)', self_loop_count)
            line_df = line_df.loc[~self_loop_mask]

        num_parallel = line_df['num_parallel'] if 'num_parallel' in line_df.columns else 1

        # Validate line types and apply voltage-level-aware fallbacks.
        valid_line_types = set(network.line_types.index)
        line_types = line_df['type'].copy()
        for idx, ltype in line_types.items():
            if ltype not in valid_line_types:
                bus0 = line_df.at[idx, 'bus0']
                bus1 = line_df.at[idx, 'bus1']
                v0 = float(df_bus.at[bus0, 'v_nom']) if bus0 in df_bus.index else 0.0
                v1 = float(df_bus.at[bus1, 'v_nom']) if bus1 in df_bus.index else 0.0
                fallback = DEFAULT_LINE_TYPE_MV if max(v0, v1) >= 1.0 else DEFAULT_LINE_TYPE_LV

                if fallback not in valid_line_types and len(valid_line_types) > 0:
                    fallback = sorted(valid_line_types)[0]

                log.warning('Line type "%s" not found for %s, using "%s"', ltype, idx, fallback)
                line_types.at[idx] = fallback

        # Ensure line lengths are positive finite values.
        lengths = pd.to_numeric(line_df['length'], errors='coerce')
        invalid_length_mask = lengths.isna() | (lengths <= 0)
        if invalid_length_mask.any():
            invalid_count = int(invalid_length_mask.sum())
            log.warning('Found %d lines with invalid length; setting to 0.001', invalid_count)
            lengths.loc[invalid_length_mask] = 0.001

        if line_df.empty:
            log.warning('Not added: all line entries were invalid after preprocessing')
        else:
            network.add(
                "Line",
                line_df.index,
                bus0=line_df['bus0'],
                bus1=line_df['bus1'],
                type=line_types,
                length=lengths,
                num_parallel=num_parallel
            )
            log.info('Added %d Lines', len(line_df))
    else:
        log.warning('Not added: PyPSA network has no lines')
    #------------------------------------------------------------------------------------------------------
    # Add generators (vectorized)
    if not df_gen.empty:
        control = df_gen['control'] if 'control' in df_gen.columns else 'PQ'
        network.add(
            "Generator",
            df_gen.index,
            bus=df_gen['bus'],
            control=control
        )
        # Set time series for generators (vectorized assignment)
        if not gen_pset.empty:
            common_gens = [g for g in df_gen.index if g in gen_pset.columns]
            if common_gens:
                network.generators_t.p_set = gen_pset[common_gens].set_index(network.snapshots)
        log.info('Added %d Generators', len(df_gen))
    else:
        log.info('Not added: PyPSA network has no generators')
    #------------------------------------------------------------------------------------------------------
    # Add loads (vectorized)
    if not df_load.empty:
        network.add(
            "Load",
            df_load.index,
            bus=df_load['bus']
        )
        # Set time series for loads (vectorized assignment)
        if not load_pset.empty:
            common_loads = [l for l in df_load.index if l in load_pset.columns]
            if common_loads:
                network.loads_t.p_set = load_pset[common_loads].set_index(network.snapshots)
        log.info('Added %d Loads', len(df_load))
    else:
        log.info('Not added: PyPSA network has no loads')
        
    return network
