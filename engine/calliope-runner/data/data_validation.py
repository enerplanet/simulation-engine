import csv
import typing
from dateutil import parser as dt_parser
from datetime import timedelta
import os
import re

import typing

# #############################################################################
# Loggin
# #############################################################################

class FileLog():

    def __init__(self, name: str) -> None:
        self.__name: str = name
        self.__logs: typing.List[str] = []

    @property
    def name(self) -> str:
        return self.__name

    @property
    def logs(self) -> typing.List[str]:
        return self.__logs

    def add(self, log: str) -> None:
        self.__logs.append(log)

    def __str__(self) -> str:
        return  'Filename: ' + self.name + '\n' + \
                ''.join([str(nr) + ': ' + log + '\n' for nr, log in enumerate(self.logs)]) + '\n'

class Logger():

    def __init__(self) -> None:
        self.__file_logs: typing.List[FileLog] = []

    @property
    def file_logs(self) -> typing.List[FileLog]:
        return self.__file_logs

    def add(self, file_log: FileLog) -> None:
        self.__file_logs.append(file_log)

    def new(self, name: str, logs: typing.List[str]) -> None:
        file_log = FileLog(name)
        for log in logs:
            file_log.add(log)
        self.add(file_log)

    def __str__(self) -> str:
        return  '# ###################################### \n' + \
                '# ###          File logs             ### \n' + \
                '# ###################################### \n' + \
                ''.join([str(file_log) for file_log in self.file_logs]) + \
                '# ###################################### \n'

# #############################################################################
# Finding all .csv files in directory
# #############################################################################

def find_files(path: str) -> typing.List[str]:
    return os.listdir(path)

def csv_filter(file: str) -> bool:
    return re.match('.*(.csv)$', file)

def find_csv_files(path: str) -> typing.List[str]:
    files = find_files(path)
    return list(filter(csv_filter, files))

# #############################################################################
# Checking the csv file
# #############################################################################

def check_csv_file(filename: str, logger: Logger) -> None:
    file_log = FileLog(filename)
    with open(filename) as csv_file:
        csv_reader_object = csv.reader(csv_file, delimiter=',')
        current_time = None
        last_time = None
        for rnr, row in enumerate(csv_reader_object):
            if len(row) < 2:
                file_log.add("Error: There must be at least 2 coloums!")
                break
            if rnr == 0:
                continue
            last_time = current_time
            current_time = row[0]
            if current_time is None or last_time is None:
                continue
            if not check_time_series_consistency(current_time, last_time, 60):
                file_log.add("Error: Timedelta missmatch at " + str(rnr) + "!")
            for cnr, col in enumerate(row):
                if cnr == 0:
                    continue
                if not check_value_consistency(col):
                    file_log.add("Error: Value '" + col + "' is no Number at " + str(rnr) + "," + str(cnr) + "!")
    logger.add(file_log)

def check_time_series_consistency(current_time: str, last_time: str, delta_in_min: int) -> bool:
    dt_current_time = dt_parser.parse(current_time)
    dt_last_time = dt_parser.parse(last_time)
    dt_delta = dt_current_time - dt_last_time
    return dt_delta == timedelta(minutes=delta_in_min)
        

def check_value_consistency(val: str) -> bool:
    try:
        float(val)
        return True
    except ValueError:
        return False

# #############################################################################
# Main
# #############################################################################

def main():
    logger = Logger()
    csv_files = find_csv_files('./')
    for file in csv_files:
       check_csv_file(file, logger)
    print(logger) 

if __name__ == '__main__':
    main()