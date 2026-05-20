"""
Camply General Utilities
"""

import datetime
import logging
import sys
from datetime import date
from typing import Any, Callable, Iterable, List, Optional, Set, Tuple, Union

from camply import SearchWindow
from camply.containers.base_container import CamplyModel

logger = logging.getLogger(__name__)

ListLike = Union[List[Any], Set[Any], Tuple[Any]]


def is_list_like(obj: Any) -> bool:
    """
    Define if an object is list-like
    """
    return isinstance(obj, (list, set, tuple))


def make_list(
    obj: Any, coerce: Optional[Callable[[Any], Any]] = None
) -> Optional[List[Any]]:
    """
    Make Anything An Iterable Instance

    Parameters
    ----------
    obj: object
    coerce: Callable

    Returns
    -------
    List[object]
    """
    if obj is None:
        return None
    elif isinstance(obj, CamplyModel):
        return [coerce(obj) if coerce is not None else obj]
    elif is_list_like(obj) is True:
        if coerce is not None:
            return [coerce(item) for item in obj]
        else:
            return list(obj)
    else:
        return [coerce(obj) if coerce is not None else obj]


def handle_search_windows(
    start_date: Union[Iterable[str], str, Iterable[datetime.date], datetime.date],
    end_date: Union[Iterable[str], str, Iterable[datetime.date], datetime.date],
) -> Union[List[SearchWindow], SearchWindow]:
    """
    Handle Multiple Search Windows by the CLI
    """
    start_dates: List[Union[str, date]] = []
    end_dates: List[Union[str, date]] = []

    if isinstance(start_date, (str, date)):
        start_dates = [start_date]
        assert isinstance(end_date, (str, date))
        end_dates = [end_date]
    else:
        start_dates = list(start_date)
        end_dates = list(end_date)  # type: ignore[arg-type]
    search_windows: List[SearchWindow] = []
    for field in [start_dates, end_dates]:
        if field is None or (isinstance(field, (tuple, list)) and len(field) == 0):
            logger.error("Campsite searches require a `start_date` and an `end_date`")
            sys.exit(1)
    if len(start_dates) != len(end_dates):
        logger.error(
            "When searching multiple date windows, you must provide the same amount "
            "of `--start-dates` as `--end-dates`"
        )
        sys.exit(1)
    for index, date_str in enumerate(start_dates):
        end_date_str = end_dates[index]
        if isinstance(date_str, str):
            date_str = datetime.datetime.strptime(date_str, "%Y-%m-%d").date()
        if isinstance(end_date_str, str):
            end_date_str = datetime.datetime.strptime(end_date_str, "%Y-%m-%d").date()
        search_windows.append(SearchWindow(start_date=date_str, end_date=end_date_str))
    if len(search_windows) == 1:
        return search_windows[0]
    else:
        return search_windows


days_of_the_week_base = {
    "Monday": 0,
    "Tuesday": 1,
    "Wednesday": 2,
    "Thursday": 3,
    "Friday": 4,
    "Saturday": 5,
    "Sunday": 6,
}
days_of_the_week_mapping = days_of_the_week_base.copy()
days_of_the_week_mapping.update(
    {
        "MON": 0,
        "TUE": 1,
        "TUES": 1,
        "WED": 2,
        "THU": 3,
        "THUR": 3,
        "THURS": 3,
        "FRI": 4,
        "SAT": 5,
        "SUN": 6,
    }
)
