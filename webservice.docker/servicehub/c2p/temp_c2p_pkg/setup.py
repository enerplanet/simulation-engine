from setuptools import setup, find_packages

setup(
    name="c2p",
    version="1.0.5",
    packages=find_packages(),
    include_package_data=True,
    package_data={
        'c2p': ['defaults/*.csv'],
    },
    install_requires=[
        'pandas',
        'numpy',
        'pypsa',
        'xarray',
        'netCDF4',
    ],
)
