"""
Calliope to PyPSA
"""

import logging

import numpy as np
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
        self.converged = False
        self.convergence_stats = {}


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
                except Exception as exc:
                    log.warning('Failed to remove invalid PF export %s: %s', file_path, exc)

    def _remove_components(self, component, names):
        """
        Remove network components one by one with safe logging.
        """
        if self.net is None:
            return

        for name in list(names):
            try:
                self.net.remove(component, name)
                log.warning('Removed invalid %s "%s"', component, name)
            except Exception as exc:
                log.warning('Failed to remove %s "%s": %s', component, name, exc)

    def _add_synthetic_slack(self, bus, sub_network):
        """
        Add a synthetic slack generator to anchor generator-less subnetworks.
        """
        if self.net is None or bus not in self.net.buses.index:
            return None

        base_name = f'synthetic_slack_sn_{sub_network}'
        name = base_name
        i = 1
        while name in self.net.generators.index:
            i += 1
            name = f'{base_name}_{i}'

        self.net.add("Generator", name, bus=bus, control='Slack')

        # Ensure deterministic time-series defaults for the synthetic slack.
        if 'p_set' in self.net.generators_t:
            self.net.generators_t.p_set[name] = 0.0
        if 'q_set' in self.net.generators_t:
            self.net.generators_t.q_set[name] = 0.0

        log.warning(
            'Added synthetic slack generator %s on bus %s for sub-network %s',
            name,
            bus,
            sub_network,
        )
        return name

    def _validate_network(self):
        """
        Validate network topology before power flow.
        Returns True if valid, False otherwise.
        """
        issues = []
        
        # 1. Check for isolated buses (buses with no connections)
        if self.net is not None:
            all_buses = set(self.net.buses.index)
            connected_buses = set()
            
            # Buses connected via lines
            if len(self.net.lines) > 0:
                connected_buses.update(self.net.lines.bus0.values)
                connected_buses.update(self.net.lines.bus1.values)
            
            # Buses connected via transformers
            if len(self.net.transformers) > 0:
                connected_buses.update(self.net.transformers.bus0.values)
                connected_buses.update(self.net.transformers.bus1.values)
            
            isolated = all_buses - connected_buses
            if isolated:
                log.warning('Found %d isolated buses: %s', len(isolated), list(isolated)[:5])
                issues.append(f'{len(isolated)} isolated buses')

            # Remove self-loop branches (bus0 == bus1) which can produce singular Jacobians.
            loop_lines = self.net.lines[self.net.lines.bus0 == self.net.lines.bus1].index
            if len(loop_lines) > 0:
                issues.append(f'{len(loop_lines)} self-loop lines (removed)')
                self._remove_components("Line", loop_lines)

            loop_trafos = self.net.transformers[self.net.transformers.bus0 == self.net.transformers.bus1].index
            if len(loop_trafos) > 0:
                issues.append(f'{len(loop_trafos)} self-loop transformers (removed)')
                self._remove_components("Transformer", loop_trafos)
            
            # 2. Check transformer voltage levels (bus0 should be higher voltage)
            for idx, trafo in self.net.transformers.iterrows():
                bus0_v = self.net.buses.loc[trafo.bus0, 'v_nom'] if trafo.bus0 in self.net.buses.index else 0
                bus1_v = self.net.buses.loc[trafo.bus1, 'v_nom'] if trafo.bus1 in self.net.buses.index else 0
                if bus0_v < bus1_v:
                    log.warning('Transformer %s: bus0 (%s, %.1f kV) < bus1 (%s, %.1f kV) - swapping recommended', 
                               idx, trafo.bus0, bus0_v, trafo.bus1, bus1_v)
                    issues.append(f'Trafo {idx} voltage order incorrect')
            
            # 3. Check each sub-network has a slack generator
            self.net.determine_network_topology()
            for sn in self.net.sub_networks.index:
                sn_buses = self.net.buses[self.net.buses.sub_network == sn].index
                sn_gens = self.net.generators[self.net.generators.bus.isin(sn_buses)]
                slack_gens = sn_gens[sn_gens.control == 'Slack']
                if len(slack_gens) == 0:
                    log.warning('Sub-network %s has no slack generator - adding one', sn)
                    # Auto-add slack: find first generator and make it slack
                    if len(sn_gens) > 0:
                        first_gen = sn_gens.index[0]
                        self.net.generators.loc[first_gen, 'control'] = 'Slack'
                        log.info('Set generator %s as Slack for sub-network %s', first_gen, sn)
                    else:
                        issues.append(f'Sub-network {sn} has no generators')
            
            # 4. Check voltage levels consistency
            lv_buses = self.net.buses[self.net.buses.v_nom < 1.0]  # < 1 kV
            mv_buses = self.net.buses[(self.net.buses.v_nom >= 1.0) & (self.net.buses.v_nom < 50)]
            
            # LV should be 0.4 kV
            wrong_lv = lv_buses[lv_buses.v_nom != 0.4]
            if len(wrong_lv) > 0:
                log.warning('%d LV buses have non-standard voltage (should be 0.4 kV)', len(wrong_lv))
            
            # MV should be 10 or 20 kV
            wrong_mv = mv_buses[~mv_buses.v_nom.isin([10, 20])]
            if len(wrong_mv) > 0:
                log.warning('%d MV buses have non-standard voltage (should be 10 or 20 kV)', len(wrong_mv))
            
            # 5. Check for zero-impedance transformers and fix
            zero_trafos = self.net.transformers[
                self.net.transformers.x.isna()
                | self.net.transformers.r.isna()
                | (self.net.transformers.x == 0)
                | (self.net.transformers.s_nom <= 0)
                | self.net.transformers.s_nom.isna()
            ]
            if len(zero_trafos) > 0:
                log.warning('Found %d zero-impedance transformers - fixing with default values', len(zero_trafos))
                for idx in zero_trafos.index:
                    # Set reasonable default values for 20/0.4 kV transformer
                    self.net.transformers.loc[idx, 'x'] = 0.04  # 4% reactance
                    self.net.transformers.loc[idx, 'r'] = 0.01  # 1% resistance
                    self.net.transformers.loc[idx, 's_nom'] = 0.4  # 400 kVA default
                    log.info('Fixed zero-impedance transformer %s: x=0.04, r=0.01, s_nom=0.4', idx)
                issues.append(f'{len(zero_trafos)} zero-impedance transformers (fixed)')
            
            # 6. Check for zero-impedance lines and fix
            zero_lines = self.net.lines[
                self.net.lines.x.isna()
                | self.net.lines.r.isna()
                | ((self.net.lines.x == 0) & (self.net.lines.r == 0))
            ]
            if len(zero_lines) > 0:
                log.warning('Found %d zero-impedance lines - fixing with default values', len(zero_lines))
                for idx in zero_lines.index:
                    # Set minimal impedance (short cable)
                    self.net.lines.loc[idx, 'x'] = 0.001  # Small reactance
                    self.net.lines.loc[idx, 'r'] = 0.001  # Small resistance
                    log.info('Fixed zero-impedance line %s: x=0.001, r=0.001', idx)
                issues.append(f'{len(zero_lines)} zero-impedance lines (fixed)')
        
        self.convergence_stats['validation_issues'] = issues
        return len(issues) == 0

    def _ensure_slack_per_subnetwork(self):
        """
        Ensure each sub-network has exactly one slack bus.
        """
        if self.net is None:
            return
        
        self.net.determine_network_topology()
        
        for sn_idx in self.net.sub_networks.index:
            sn_buses = self.net.buses[self.net.buses.sub_network == sn_idx].index.tolist()
            sn_gens = self.net.generators[self.net.generators.bus.isin(sn_buses)]
            
            slack_count = (sn_gens.control == 'Slack').sum()
            
            if slack_count == 0:
                # No slack - find best candidate (prefer transformer_supply, then largest generator)
                if len(sn_gens) > 0:
                    # Prefer transformer_supply
                    trafo_gens = sn_gens[sn_gens.index.str.contains('transformer', case=False)]
                    if len(trafo_gens) > 0:
                        best_gen = trafo_gens.index[0]
                    else:
                        # Use first generator
                        best_gen = sn_gens.index[0]
                    
                    self.net.generators.loc[best_gen, 'control'] = 'Slack'
                    log.info('Added slack generator %s to sub-network %s', best_gen, sn_idx)
                elif len(sn_buses) > 0:
                    # Generator-less island: add synthetic slack to prevent singular Jacobian.
                    self._add_synthetic_slack(sn_buses[0], sn_idx)
            elif slack_count > 1:
                # Multiple slacks - keep only first one
                slack_gens = sn_gens[sn_gens.control == 'Slack'].index.tolist()
                for gen in slack_gens[1:]:
                    self.net.generators.loc[gen, 'control'] = 'PV'
                    log.info('Changed excess slack %s to PV in sub-network %s', gen, sn_idx)

    def _log_power_balance(self):
        """Log power balance information for debugging."""
        if self.net is None:
            return
        
        # Log network summary
        log.info('Network: %d buses, %d lines, %d transformers, %d generators, %d loads',
                len(self.net.buses), len(self.net.lines), len(self.net.transformers),
                len(self.net.generators), len(self.net.loads))
        
        # Log transformer info
        for idx, trafo in self.net.transformers.iterrows():
            log.info('Transformer %s: type=%s, s_nom=%.3f MVA, x=%.4f, r=%.4f',
                    idx, trafo.type, trafo.s_nom, trafo.x, trafo.r)
        
        # Log power balance per sub-network
        self.net.determine_network_topology()
        for sn_idx in self.net.sub_networks.index:
            sn_buses = self.net.buses[self.net.buses.sub_network == sn_idx].index.tolist()
            sn_gens = self.net.generators[self.net.generators.bus.isin(sn_buses)]
            sn_loads = self.net.loads[self.net.loads.bus.isin(sn_buses)]
            
            # Get power at first snapshot
            if 'p_set' in self.net.generators_t and len(self.net.generators_t.p_set) > 0:
                gen_p = self.net.generators_t.p_set.iloc[0][sn_gens.index].sum()
            else:
                gen_p = 0
            
            if 'p_set' in self.net.loads_t and len(self.net.loads_t.p_set) > 0:
                load_p = self.net.loads_t.p_set.iloc[0][sn_loads.index].sum()
            else:
                load_p = 0
            
            slack_count = (sn_gens.control == 'Slack').sum()
            log.info('Sub-network %s: %d buses, %d gens (%d slack), %d loads, Gen=%.2f MW, Load=%.2f MW, Mismatch=%.2f MW',
                    sn_idx, len(sn_buses), len(sn_gens), slack_count, len(sn_loads), gen_p, load_p, gen_p - load_p)
        
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
        
        # Validate network topology
        self._validate_network()
        
        # Ensure each sub-network has a slack bus
        self._ensure_slack_per_subnetwork()
        
        # Log power balance for debugging
        self._log_power_balance()
        
        # Consistency check
        if self.settings.con_check:
            log.info('Consistency check')
            self.net.consistency_check()
        
        # Run linear power flow first (fast)
        log.info('Running linear power flow (lpf)')
        lpf_result = self.net.lpf()
        
        # Check lpf results for large mismatches
        if hasattr(self.net, 'buses_t') and 'p' in self.net.buses_t:
            max_mismatch = self.net.buses_t.p.abs().max().max() if len(self.net.buses_t.p) > 0 else 0
            log.info('LPF max power mismatch: %.2f MW', max_mismatch)
            self.convergence_stats['lpf_max_mismatch'] = float(max_mismatch)
        
        # Run non-linear power flow
        # Always use seed=True after lpf() to improve NR convergence
        seed = True
        num_snapshots = len(self.net.snapshots)
        pf_attempts = [
            {
                'label': 'distributed_slack=False',
                'kwargs': {'use_seed': seed, 'x_tol': 1e-4, 'distribute_slack': False},
            },
        ]

        self.full_pf = None
        pf_errors = []
        for attempt in pf_attempts:
            label = attempt['label']
            try:
                log.info(
                    'Running non-linear power flow (pf) [%s] with x_tol=1e-4 and use_seed=True on %d snapshots',
                    label,
                    num_snapshots,
                )
                self.full_pf = self.net.pf(**attempt['kwargs'])
                self.convergence_stats['pf_attempt'] = label
                break
            except Exception as e:
                pf_errors.append(f'{label}: {e}')
                log.error('network.pf() failed [%s]: %s', label, e)
                # Refresh slack assignment before next retry.
                self._ensure_slack_per_subnetwork()

        # Check convergence - result['converged'] is a DataFrame with snapshots as rows
        if self.full_pf is not None and 'converged' in self.full_pf:
            converged_df = self.full_pf['converged']
            converged_count = int(converged_df.values.sum())
            total_count = int(converged_df.size)
            self.converged = converged_count == total_count
            self.convergence_stats['converged_snapshots'] = converged_count
            self.convergence_stats['total_snapshots'] = total_count
            log.info('Power flow converged: %d/%d snapshots', converged_count, total_count)
            
            # Reject PF-derived exports if any snapshot failed to converge.
            if converged_count < total_count:
                log.error('Power flow failed to converge for some or all snapshots. Clearing results.')
                self.full_pf = None
                self.converged = False

            # Early termination check - if first 3 snapshots fail, likely all will fail
            if total_count >= 3 and converged_count == 0:
                log.warning('First snapshots did not converge - network may have topology issues')
        else:
            self.converged = self.full_pf is not None
            if self.full_pf is None:
                self.convergence_stats['error'] = ' | '.join(pf_errors) if pf_errors else 'network.pf() returned None'
                log.error('network.pf() returned None after all retries')

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
                snapshots = None
                if self.net is not None and len(self.net.snapshots) > 0:
                    snapshots = list(self.net.snapshots)
                else:
                    try:
                        timesteps = self.get.timesteps()
                        if len(timesteps) > 0:
                            snapshots = list(timesteps)
                    except Exception:
                        snapshots = None

                if not snapshots:
                    snapshots = [0]

                failed_pf_df = pd.DataFrame({
                    'snapshots': snapshots,
                    'n_iter': [0] * len(snapshots),
                    'converged': [False] * len(snapshots),
                    'error': ['pf_failed'] * len(snapshots),
                })
                ex.df(failed_pf_df, self.path.output_dir, self.path.pf_result_name, formats, True)
                log.info(
                    'Saved fallback pf_result with converged=False for %d snapshots',
                    len(snapshots),
                )

    def save_settings(self):
        """
        Saves all settings.
        """ 

        self.settings.save()
        
        # Also save convergence stats
        if self.convergence_stats:
            stats_df = pd.DataFrame([self.convergence_stats])
            stats_path = self.path.output_dir / 'convergence_stats.csv'
            stats_df.to_csv(stats_path, index=False)
            log.info('Saved convergence stats to %s', stats_path)

    def save(self):
        """
        Saves all available data.
        """
        
        self.save_net()
        self.save_pf_results()
        self.save_settings()
