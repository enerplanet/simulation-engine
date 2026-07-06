#!/usr/bin/env python3
"""
Generate BDEW electricity SLP CSV profiles and a per-f_class index.

This generator produces:
- canonical profile CSV files (H25, G25, G1, G2, G3, G4, G5, G6, L25)
- legacy pylovo aliases (SFH, TH, MFH, AB, Commercial, Public, Industrial, Agricultural)
- demand_index.json entries for both legacy classes and granular f_class values
- f_class_profile_mapping.csv (audit table: f_class -> chosen BDEW profile)

The f_class list is built from:
1) enerplanet-react slpByFClass map (if available), and
2) pylovo CONSUMER_CATEGORIES definitions (if available).
"""

from __future__ import annotations

import csv
import json
import os
import re
import shutil
import sys
from pathlib import Path
from typing import Dict, Iterable, List, Set, Tuple

import numpy as np
import pandas as pd

try:
    from demandlib import bdew
    HAS_DEMANDLIB = True
except ImportError:
    HAS_DEMANDLIB = False
    print("ERROR: demandlib not installed. Install with: pip install demandlib")
    sys.exit(1)


# BDEW profile descriptions used in logs and index metadata.
BDEW_PROFILES = {
    "h0": "Residential (Haushalt) - Original",
    "g0": "Commercial general (Gewerbe allgemein)",
    "g1": "Commercial daytime weekdays (Gewerbe werktags 08-18)",
    "g2": "Commercial with evening focus (Gewerbe Abendanteil)",
    "g3": "Commercial/industrial continuous operation (Dauerbetrieb)",
    "g4": "Retail/shop profile (Laden/Friseur)",
    "g5": "Bakery with production (Baeckerei mit Backstube)",
    "g6": "Commercial with weekend operation (Wochenendbetrieb)",
    "l0": "Agricultural general (Landwirtschaft)",
    "l1": "Agricultural dairy (Landwirtschaft Milchwirtschaft)",
    "l2": "Agricultural other (Landwirtschaft sonstige)",
    "h25": "Residential (Haushalt) - BDEW 2025",
    "g25": "Commercial (Gewerbe) - BDEW 2025",
    "l25": "Agricultural (Landwirtschaft) - BDEW 2025",
}

# Canonical profile CSV files generated once each.
PROFILE_OUTPUT_BASENAME = {
    "h25": "H25",
    "g25": "G25",
    "g1": "G1",
    "g2": "G2",
    "g3": "G3",
    "g4": "G4",
    "g5": "G5",
    "g6": "G6",
    "l25": "L25",
}

# Backward-compatible pylovo building keys.
LEGACY_BUILDING_TYPE_PROFILE = {
    "SFH": "h25",
    "TH": "h25",
    "MFH": "h25",
    "AB": "h25",
    "Commercial": "g25",
    "Public": "g1",
    "Industrial": "g3",
    "Agricultural": "l25",
}

# Mapping from legacy SLP bucket to the chosen BDEW profile family.
BASE_SLP_TO_PROFILE = {
    "sfh": "h25",
    "th": "h25",
    "mfh": "h25",
    "ab": "h25",
    "commercial": "g25",
    "public": "g1",
    "industrial": "g3",
    "agricultural": "l25",
}

# High-confidence class-specific overrides.
OVERRIDE_PROFILE_BY_FCLASS = {
    "bakery": "g5",
    "bakehouse": "g5",
    "restaurant": "g2",
    "cafe": "g2",
    "bar": "g2",
    "pub": "g2",
    "fast_food": "g2",
    "nightclub": "g2",
    "hotel": "g6",
    "motel": "g6",
    "hostel": "g6",
    "guest_house": "g6",
    "shop": "g4",
    "retail": "g4",
    "store": "g4",
    "supermarket": "g4",
    "mall": "g4",
    "convenience": "g4",
    "kiosk": "g4",
}

DEFAULT_ENERPLANET_SLP_MAP_PATH = Path(
    "/home/askhan/Documents/Development/enerplanet-react/enerplanet/backend/internal/payload/payload_building_helpers.go"
)
DEFAULT_PYLOVO_CONFIG_PATH = Path(
    "/home/askhan/Documents/Development/pylovo/config/config_generation.yaml"
)


def normalize_fclass(value: str) -> str:
    """Normalize f_class tokens to a predictable key format."""
    v = (value or "").strip().lower()
    if not v:
        return ""
    v = v.replace("-", "_").replace(" ", "_")
    while "__" in v:
        v = v.replace("__", "_")
    return v.strip("_")


def parse_enerplanet_slp_map(path: Path) -> Dict[str, str]:
    """Parse slpByFClass map from enerplanet-react Go source."""
    mapping: Dict[str, str] = {}
    if not path.exists():
        return mapping

    start_re = re.compile(r"^\s*var\s+slpByFClass\s*=\s*map\[string\]string\s*{")
    pair_re = re.compile(r'^\s*"([^"]+)"\s*:\s*"([^"]+)"\s*,?\s*$')
    in_map = False

    for line in path.read_text(encoding="utf-8").splitlines():
        if not in_map:
            if start_re.search(line):
                in_map = True
            continue
        if line.strip().startswith("}"):
            break
        m = pair_re.match(line)
        if not m:
            continue
        f_class = normalize_fclass(m.group(1))
        slp_class = normalize_fclass(m.group(2))
        if f_class:
            mapping[f_class] = slp_class
    return mapping


def parse_pylovo_definitions(path: Path) -> Set[str]:
    """Extract f_class definitions from pylovo CONSUMER_CATEGORIES YAML lines."""
    values: Set[str] = set()
    if not path.exists():
        return values

    line_re = re.compile(r"definition:\s*([\"']?)([A-Za-z0-9_]+)\1")
    for line in path.read_text(encoding="utf-8").splitlines():
        m = line_re.search(line)
        if not m:
            continue
        fc = normalize_fclass(m.group(2))
        if fc:
            values.add(fc)
    return values


def infer_profile_from_keywords(f_class: str) -> str:
    """Fallback profile selection for unseen f_class tokens."""
    fc = normalize_fclass(f_class)
    if not fc:
        return "g25"

    if fc in OVERRIDE_PROFILE_BY_FCLASS:
        return OVERRIDE_PROFILE_BY_FCLASS[fc]

    if any(k in fc for k in ("house", "apartment", "residential", "dormitory", "terrace", "townhouse", "bungalow", "villa")):
        return "h25"
    if any(k in fc for k in ("farm", "barn", "greenhouse", "agricultural", "stable", "livestock", "cowshed", "hayloft", "granary", "coop")):
        return "l25"
    if any(k in fc for k in ("industrial", "factory", "warehouse", "workshop", "manufacture", "plant", "substation", "utility", "station", "terminal", "power", "data_center")):
        return "g3"
    if any(k in fc for k in ("school", "kindergarten", "university", "college", "government", "community", "library", "museum", "theatre", "church", "hospital", "clinic")):
        return "g1"
    if any(k in fc for k in ("shop", "retail", "market", "supermarket", "store", "mall", "kiosk", "pharmacy", "clothes", "shoes", "books", "electronics", "furniture", "hardware")):
        return "g4"
    if any(k in fc for k in ("restaurant", "cafe", "bar", "pub", "nightclub", "fast_food")):
        return "g2"
    if any(k in fc for k in ("hotel", "hostel", "motel", "guest_house")):
        return "g6"

    return "g25"


def pick_profile_for_fclass(f_class: str, base_slp_class: str | None) -> Tuple[str, str]:
    """
    Select BDEW profile type and reason for one f_class.

    Priority:
    1) explicit override
    2) base slpByFClass bucket mapping
    3) keyword fallback
    """
    fc = normalize_fclass(f_class)
    if not fc:
        return "g25", "default_empty"

    if fc in OVERRIDE_PROFILE_BY_FCLASS:
        return OVERRIDE_PROFILE_BY_FCLASS[fc], "override"

    base = normalize_fclass(base_slp_class or "")
    if base in BASE_SLP_TO_PROFILE:
        return BASE_SLP_TO_PROFILE[base], f"slp_bucket:{base}"

    inferred = infer_profile_from_keywords(fc)
    return inferred, "keyword_fallback"


def build_fclass_profile_mapping(
    enerplanet_slp_map_path: Path,
    pylovo_config_path: Path,
) -> Tuple[Dict[str, str], Dict[str, Dict[str, str]]]:
    """
    Build f_class -> profile mapping and a metadata audit table.
    """
    slp_map = parse_enerplanet_slp_map(enerplanet_slp_map_path)
    pylovo_defs = parse_pylovo_definitions(pylovo_config_path)

    class_set: Set[str] = set(slp_map.keys()) | set(pylovo_defs)
    class_set.update({
        "default",
        "_default",
        "residential",
        "commercial",
        "public",
        "industrial",
        "agricultural",
        "house",
        "apartments",
        "bakery",
        "kindergarten",
    })

    mapping: Dict[str, str] = {}
    meta: Dict[str, Dict[str, str]] = {}

    for fc in sorted(class_set):
        profile_type, reason = pick_profile_for_fclass(fc, slp_map.get(fc))
        mapping[fc] = profile_type
        meta[fc] = {
            "profile_type": profile_type,
            "profile_desc": BDEW_PROFILES.get(profile_type, profile_type),
            "base_slp_class": slp_map.get(fc, ""),
            "selection_reason": reason,
        }

    return mapping, meta


def write_fclass_mapping_csv(meta: Dict[str, Dict[str, str]], output_dir: Path) -> Path:
    output_path = output_dir / "f_class_profile_mapping.csv"
    rows = []
    for f_class in sorted(meta.keys()):
        info = meta[f_class]
        rows.append(
            {
                "f_class": f_class,
                "bdew_profile": info["profile_type"].upper(),
                "bdew_profile_description": info["profile_desc"],
                "source_slp_class": info["base_slp_class"],
                "selection_reason": info["selection_reason"],
                "profile_file": f"{PROFILE_OUTPUT_BASENAME[info['profile_type']]}.csv",
            }
        )

    with output_path.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(
            handle,
            fieldnames=[
                "f_class",
                "bdew_profile",
                "bdew_profile_description",
                "source_slp_class",
                "selection_reason",
                "profile_file",
            ],
        )
        writer.writeheader()
        writer.writerows(rows)

    return output_path


def generate_bdew_profile(year: int, profile_type: str) -> pd.Series:
    """
    Generate normalized hourly BDEW load profile for one year.
    """
    time_index = pd.date_range(
        start=f"{year}-01-01 00:00",
        end=f"{year}-12-31 23:45",
        freq="15min",
    )

    if profile_type in ("h25", "g25", "l25"):
        if profile_type == "h25":
            from demandlib.bdew import H25
            profile = H25(time_index)
        elif profile_type == "g25":
            from demandlib.bdew import G25
            profile = G25(time_index)
        else:
            from demandlib.bdew import L25
            profile = L25(time_index)

        if isinstance(profile, pd.Series):
            profile = profile / 1_000_000
        else:
            profile = pd.Series(profile, index=time_index) / 1_000_000
    else:
        e_slp = bdew.ElecSlp(year, holidays=None)
        demand = {profile_type: 1.0}
        elec_demand = e_slp.get_scaled_power_profiles(demand, conversion_factor=4)
        profile = elec_demand[profile_type]

    profile = profile.resample("h").mean()
    total = float(profile.sum())
    if total > 0:
        profile = profile / total
    return profile


def create_demand_levels(min_demand: int = 50, max_demand: int = 200000) -> List[int]:
    """
    Build mixed-resolution demand columns (kWh/year).
    """
    demands = set()
    demands.update(range(50, 500, 20))
    demands.update(range(500, 5000, 50))
    demands.update(range(5000, 20000, 200))
    demands.update(range(20000, 50000, 500))
    demands.update(range(50000, max_demand + 1, 2000))
    demands.add(min_demand)
    demands.add(max_demand)
    return sorted(demands)


def generate_csv(
    profile_type: str,
    output_name: str,
    output_dir: Path,
    start_year: int = 2015,
    end_year: int = 2025,
    min_demand: int = 50,
    max_demand: int = 200000,
) -> Dict[str, object]:
    """
    Generate one CSV file for one BDEW profile family.
    """
    profile_desc = BDEW_PROFILES.get(profile_type, profile_type)
    print(f"Generating {output_name} ({profile_desc})...")

    demands = create_demand_levels(min_demand, max_demand)
    print(f"  - {len(demands)} demand levels: {min_demand} - {max_demand} kWh/year")

    all_profiles = []
    for year in range(start_year, end_year + 1):
        all_profiles.append(generate_bdew_profile(year, profile_type))
    combined = pd.concat(all_profiles)
    print(f"  - {len(combined)} hours ({start_year}-{end_year})")

    base_values = combined.values
    data_2d = np.zeros((len(combined.index), len(demands)), dtype=np.float32)
    for i, demand in enumerate(demands):
        data_2d[:, i] = -base_values * demand

    df = pd.DataFrame(data_2d, index=combined.index, columns=demands)
    csv_path = output_dir / f"{output_name}.csv"
    df.to_csv(csv_path)
    csv_size_mb = csv_path.stat().st_size / (1024 * 1024)
    print(f"  - Saved CSV: {csv_path} ({csv_size_mb:.1f} MB)")

    return {
        "file": f"{output_name}.csv",
        "demand_levels": demands,
        "bdew_profile": profile_type.upper(),
        "hours": len(combined.index),
    }


def generate_index_file(index_data: Dict[str, Dict[str, object]], output_dir: Path) -> Path:
    index = {
        "version": "3.0",
        "format": "csv",
        "building_types": {},
    }
    for key, data in sorted(index_data.items()):
        index["building_types"][key] = {
            "file": data["file"],
            "bdew_profile": data["bdew_profile"],
            "demand_levels": data["demand_levels"],
            "hours": data["hours"],
        }

    output_path = output_dir / "demand_index.json"
    output_path.write_text(json.dumps(index, indent=2), encoding="utf-8")
    print(f"\nIndex file saved: {output_path}")
    return output_path


def copy_alias(src: Path, dst: Path) -> None:
    """
    Create a lightweight alias where possible.
    """
    if dst.exists():
        dst.unlink()
    try:
        os.link(src, dst)
    except OSError:
        shutil.copy2(src, dst)


def main() -> None:
    script_dir = Path(__file__).resolve().parent
    output_dir = script_dir / "webservice.docker/servicehub/data"
    output_dir.mkdir(parents=True, exist_ok=True)

    print("=" * 72)
    print("BDEW Standard Load Profile Generator (CSV, f_class-aware)")
    print("=" * 72)
    print(f"Output directory: {output_dir}")
    print(f"demandlib available: {HAS_DEMANDLIB}")
    print()

    start_year = 2015
    end_year = 2025
    min_demand = 50
    max_demand = 200000

    print(f"Date range: {start_year} - {end_year} ({end_year - start_year + 1} years)")
    print(f"Demand range: {min_demand} - {max_demand} kWh/year")
    print()

    fclass_profile_map, fclass_meta = build_fclass_profile_mapping(
        enerplanet_slp_map_path=DEFAULT_ENERPLANET_SLP_MAP_PATH,
        pylovo_config_path=DEFAULT_PYLOVO_CONFIG_PATH,
    )
    mapping_csv = write_fclass_mapping_csv(fclass_meta, output_dir)
    print(f"f_class mapping table: {mapping_csv}")

    required_profiles = set(LEGACY_BUILDING_TYPE_PROFILE.values()) | set(fclass_profile_map.values())
    unknown_profiles = sorted(required_profiles - set(PROFILE_OUTPUT_BASENAME.keys()))
    if unknown_profiles:
        raise ValueError(f"Unknown profile types in mapping: {unknown_profiles}")

    canonical_data: Dict[str, Dict[str, object]] = {}
    for profile_type in sorted(required_profiles):
        output_name = PROFILE_OUTPUT_BASENAME[profile_type]
        canonical_data[profile_type] = generate_csv(
            profile_type=profile_type,
            output_name=output_name,
            output_dir=output_dir,
            start_year=start_year,
            end_year=end_year,
            min_demand=min_demand,
            max_demand=max_demand,
        )

    index_data: Dict[str, Dict[str, object]] = {}

    # Legacy building classes for backward compatibility.
    for legacy_key, profile_type in LEGACY_BUILDING_TYPE_PROFILE.items():
        src_data = canonical_data[profile_type]
        legacy_csv = output_dir / f"{legacy_key}.csv"
        canonical_csv = output_dir / src_data["file"]
        if legacy_csv != canonical_csv:
            print(f"Creating alias {legacy_key}.csv -> {src_data['file']}")
            copy_alias(canonical_csv, legacy_csv)

        index_data[legacy_key] = {
            "file": f"{legacy_key}.csv",
            "bdew_profile": src_data["bdew_profile"],
            "demand_levels": src_data["demand_levels"],
            "hours": src_data["hours"],
        }

    # Granular f_class entries used by pylovo / enerplanet-react.
    for f_class, profile_type in sorted(fclass_profile_map.items()):
        src_data = canonical_data[profile_type]
        alias_filename = f"{f_class}.csv"
        alias_csv = output_dir / alias_filename
        canonical_csv = output_dir / src_data["file"]
        if alias_csv != canonical_csv:
            copy_alias(canonical_csv, alias_csv)

        index_data[f_class] = {
            "file": alias_filename,
            "bdew_profile": src_data["bdew_profile"],
            "demand_levels": src_data["demand_levels"],
            "hours": src_data["hours"],
        }

    generate_index_file(index_data, output_dir)

    print()
    print("=" * 72)
    print("Generation complete")
    print("=" * 72)
    print()
    print("Legacy building class mapping:")
    for legacy_key, profile_type in LEGACY_BUILDING_TYPE_PROFILE.items():
        print(f"  {legacy_key:12s} -> {profile_type.upper():4s} ({BDEW_PROFILES[profile_type]})")
    print()
    print(f"Granular f_class entries indexed: {len(fclass_profile_map)}")
    print(f"Canonical profiles generated: {len(canonical_data)}")
    print()
    print("File sizes:")
    total_size = 0
    for filename in sorted(output_dir.iterdir()):
        if filename.suffix.lower() == ".csv":
            size = filename.stat().st_size
            total_size += size
            print(f"  {filename.name}: {size / (1024 * 1024):.1f} MB")
    print(f"  Total CSV size: {total_size / (1024 * 1024):.1f} MB")


if __name__ == "__main__":
    main()
