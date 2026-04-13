"""
Regular Expressions
"""

#patterns to build input dataframes for pypsa
trafo_pattern = r'(?i)trafo.*'
carrier_pattern = r'(?i)electricity|power'
gen_slack_pattern = r'(?i)transformer_supply|stadtwerke|transformer'
gen_tech_pattern = r'(?i)pv|wind|biomass|electrolyzer|hydrogen|geothermal|swh|gasturbine|chp|battery|transformer|stadtwerke|supply_*'
gen_tech_mv_pattern = r'(?i)gasturbine'
load_tech_pattern = r'(?i)h0|g0|g1|g2|g3|g4|g5|g6|g7|l0|l1|l2|ptg|p2g|battery|demand|verbrauch_*'
load_tech_mv_pattern = r'(?i)g0|g1|g2|g3|g4|g5|g6|g7|l0|l1|l2'

#patterns to set paths
hexagon_pattern = r'(?i)SIM_(\d+)'
model_id_pattern = r'(.*)<model_id>(.*)'
model_name_pattern = r'(.*)<model_name>(.*)'

#patterns to read default pre- and suffix values
suffix_trafo_bus_lv_pattern = r'^suffix_trafo_bus_lv$'
suffix_trafo_bus_mv_pattern = r'^suffix_trafo_bus_mv$'
suffix_trafo_bus_hv_pattern = r'^suffix_trafo_bus_hv$'
prefix_poi_pattern = r'^prefix_poi$'
prefix_bus_pattern = r'^prefix_bus$'
prefix_gen_pattern = r'^prefix_gen$'
suffix_gen_pattern = r'^suffix_gen$'
suffix_load_pattern = r'^suffix_load$'
prefix_load_pattern = r'^prefix_load$'
prefix_bus_id_pattern = r'^prefix_bus_id$'