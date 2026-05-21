"""
Going to Camp Web Searching Utilities
"""

import datetime
import json
import logging
import sys
from typing import Any, Dict, List, Optional, Tuple, Union

from fake_useragent import UserAgent
from pydantic import ValidationError

from camply.containers import (
    AvailableCampsite,
    AvailableResource,
    CampgroundFacility,
    RecreationArea,
)
from camply.containers.base_container import GoingToCampEquipment
from camply.containers.gtc_api_responses import ResourceLocation
from camply.exceptions import CamplyError
from camply.providers.base_provider import BaseProvider, ProviderSearchError
from camply.providers.going_to_camp.rec_areas import RECREATION_AREAS
from camply.utils import make_list
from camply.utils.logging_utils import log_sorted_response

logger = logging.getLogger(__name__)

NON_GROUP_EQUIPMENT = -32768

CAMP_SITE = -2147483648
OVERFLOW_SITE = -2147483647
GROUP_SITE = -2147483643

ENDPOINTS = {
    "CAMP_DETAILS": "https://{}/api/maps",
    "DAILY_AVAILABILITY": "https://{}/api/availability/resourcedailyavailability",
    "LIST_CAMPGROUNDS": "https://{}/api/resourceLocation",
    "LIST_EQUIPMENT": "https://{}/api/equipment",
    "LIST_RESOURCE_CATEGORY": "https://{}/api/resourcecategory",
    "LIST_RESOURCE_STATUS": "https://{}/api/availability/resourcestatus",
    "MAPDATA": "https://{}/api/availability/map",
    "SITE_DETAILS": "https://{}/api/resource/details",
    "ATTRIBUTE_DETAILS": "https://{}/api/attribute/filterable",
}


class GoingToCamp(BaseProvider):
    """
    Going To Camp API provider
    """

    @classmethod
    def find_recreation_areas(
        cls, search_string: Optional[str] = None, **kwargs: Any
    ) -> List[RecreationArea]:
        """
        Find Matching Recreation Areas based on search string

        Parameters
        ----------
        search_string: Optional[str]
            Search Keyword(s)

        Returns
        -------
        filtered_responses: List[RecreationArea]
            Array of Matching Recreation Areas
        """
        if search_string is not None:
            logger.info(f'Searching for Recreation Areas matching: "{search_string}"')

        if not search_string or search_string == "":
            rec_areas_list = list(RECREATION_AREAS.values())
            log_sorted_response(rec_areas_list)
            return rec_areas_list

        rec_areas_list = []
        for _, rec_area in RECREATION_AREAS.items():
            if (
                search_string.lower() in rec_area.recreation_area.lower()
                or search_string.lower() in rec_area.recreation_area_location.lower()
            ):
                rec_areas_list.append(rec_area)

        log_sorted_response(rec_areas_list)

        return rec_areas_list

    def rec_area_lookup(self, rec_area_id: int) -> Tuple[str, RecreationArea]:
        """
        Lookup a recreation area by ID

        Parameters
        ----------
        rec_area_id: int
            The recreation area ID to lookup

        Returns
        -------
        domain_name, rec_ara: Tuple[str, RecreationArea]
            The rec area's domain name and the recreation area object
        """
        for domain_name, rec_area in RECREATION_AREAS.items():
            if str(rec_area.recreation_area_id) == str(rec_area_id):
                return domain_name, rec_area
        raise CamplyError(f"Recreation Area ID not found: {rec_area_id}")

    def find_campgrounds(
        self,
        search_string: Optional[str] = None,
        rec_area_id: Optional[List[int]] = None,
        campground_id: Optional[List[int]] = None,
        campsite_id: Optional[List[int]] = None,
        **kwargs: Any,
    ) -> List[CampgroundFacility]:
        """
        Find Campgrounds Given a Set of Search Criteria

        Parameters
        ----------
        search_string: Optional[str]
            Search Keyword(s)
        rec_area_id: Optional[List[int]]
            Recreation Area ID by which to filter
        campground_id: Optional[List[int]]
            ID of the Campground

        Returns
        -------
        facilities: List[CampgroundFacility]
            Array of Matching Campgrounds
        """
        if rec_area_id in (None, [], ()):
            logger.error(
                "This provider requires --rec-area to be specified when seaching for campsites"
            )
            sys.exit(1)

        return self.find_facilities_per_recreation_area(
            rec_area_id=rec_area_id,
            campground_id=campground_id,
            search_string=search_string,
        )

    def _get_attr_val(self, attribute: Any, attribute_detail: Any) -> Any:
        for attr_value in attribute.get("values", []):
            for attribute_enum_detail in attribute_detail.get("values"):
                if attribute_enum_detail["enumValue"] == attr_value:
                    return _fetch_nested_key(
                        attribute_enum_detail,
                        "localizedValues",
                        0,
                        "displayName",
                    )
        return ""

    def get_site_details(self, rec_area_id: int, resource_id: int) -> Any:
        """
        Get the details about a site in a recreation area

        Parameters
        ----------
        rec_area_id: int
            Recreation Area ID by which to filter
        resource_id: int

        Returns
        -------
        details: Dict[str, str]
            The details about the site
        """
        if not hasattr(self, "_attribute_details"):
            self._attribute_details = self._api_request(
                rec_area_id, "ATTRIBUTE_DETAILS"
            )
        attribute_details: dict[str, Any] = self._attribute_details

        site_details_dict = self._api_request(
            rec_area_id,
            "SITE_DETAILS",
            {"resourceId": str(resource_id)},
        )
        if not isinstance(site_details_dict, dict):
            return {}
        site_attributes = {}
        for attribute in site_details_dict.get("definedAttributes", []):
            attr_def_id = str(attribute.get("attributeDefinitionId", ""))
            if not attr_def_id or attr_def_id not in attribute_details:
                continue
            attribute_detail = attribute_details[attr_def_id]
            attribute_name = str(
                _fetch_nested_key(
                    attribute_detail,
                    "localizedValues",
                    0,
                    "displayName",
                )
            )
            attribute_value = attribute.get("value")
            attribute_values = []
            # Attribute a multi-value enum
            if not attribute_value:
                attr_value = self._get_attr_val(attribute, attribute_detail)
                if not attr_value:
                    continue
                attribute_values.append(str(attr_value))
            else:
                attribute_values.append(f"{attribute_value}")

            site_attributes[attribute_name] = ",".join(attribute_values)
        site_details_dict["site_attributes"] = site_attributes

        return site_details_dict

    def get_reservation_link(
        self,
        rec_area_domain_name: Any,
        resource_location_id: Any,
        map_id: Any,
        equipment_id: Any,
        sub_equipment_id: Any,
        party_size: Any,
        start_date: Any,
        end_date: Any,
    ) -> Any:
        """
        Generate a URL which a site can be booked

        Returns
        -------
        url: str
            The reservation link URL

        """
        if not sub_equipment_id:
            sub_equipment_id = ""

        return (
            "https://%s/create-booking/results?mapId=%s"
            "&bookingCategoryId=0"
            "&startDate=%s"
            "&endDate=%s"
            "&isReserving=true"
            "&equipmentId=%s"
            "&subEquipmentId=%s"
            "&partySize=%s"
            "&resourceLocationId=%s"
            % (
                rec_area_domain_name,
                map_id,
                start_date.isoformat(),
                end_date.isoformat(),
                equipment_id,
                sub_equipment_id,
                party_size,
                resource_location_id,
            )
        )

    def find_facilities_per_recreation_area(  # noqa: C901
        self,
        rec_area_id: Optional[Union[List[int], int]] = None,
        campground_id: Optional[Union[List[int], int]] = None,
        search_string: Optional[str] = None,
        **kwargs: Any,
    ) -> List[CampgroundFacility]:
        """
        Find Matching Campsites by Recreation Area

        Parameters
        ----------
        rec_area_id: Optional[Union[List[int], int]]
            Recreation Area ID
        campground_id: Optional[Union[List[int], int]]
            Campground IDs
        search_string: Optional[str]
            A string to search for in the facility name

        Returns
        -------
        campgrounds: List[CampgroundFacility]
            Array of Matching Campsites
        """
        rec_area_id_list = make_list(rec_area_id, coerce=int)
        if not rec_area_id_list:
            raise CamplyError("GoingToCamp requires `rec_area_id` to be specified")

        campgrounds = []
        for specific_rec_area_id in rec_area_id_list:
            specific_rec_area_id_int = int(specific_rec_area_id)
            logger.info(
                f"Retrieving Facility Information for Recreation Area ID: `{specific_rec_area_id_int}`."
            )

            rec_area = None
            for _, ra in RECREATION_AREAS.items():
                if str(ra.recreation_area_id) == str(specific_rec_area_id_int):
                    rec_area = ra
            if not rec_area:
                logger.error(
                    f"Recreation area '{specific_rec_area_id_int}' does not exist."
                )
                sys.exit(1)

            self.campground_details = {}
            api_response = self._api_request(
                specific_rec_area_id_int, "LIST_CAMPGROUNDS"
            )

            filtered_facilities = self._filter_facilities_responses(
                specific_rec_area_id_int,
                facilities=api_response,
            )

            # Fetch campgrounds details for all facilities
            for camp_details in self._api_request(
                specific_rec_area_id_int, "CAMP_DETAILS"
            ):
                self.campground_details[
                    camp_details["resourceLocationId"]
                ] = camp_details

            # If a search string is provided, make sure every facility name contains
            # the search string
            if search_string and search_string not in [[], (), ""]:
                filtered_facilities = [
                    f
                    for f in filtered_facilities
                    if search_string.lower() in f.resource_location_name.lower()
                ]

            for facility in filtered_facilities:
                _, campground_facility = self._process_facilities_responses(
                    rec_area, facility=facility
                )
                if not campground_facility:
                    continue
                if not campground_id:
                    campgrounds.append(campground_facility)
                campground_strings = make_list(campground_id, coerce=str) or []
                if (
                    campground_id
                    and str(campground_facility.facility_id) in campground_strings
                ):
                    campgrounds.append(campground_facility)
        logger.info(f"{len(campgrounds)} Matching Campgrounds Found")
        log_sorted_response(response_array=campgrounds)
        return campgrounds

    def _hostname_for(self, recreation_area_id: int) -> Optional[str]:
        for hostname, recreation_area in RECREATION_AREAS.items():
            if str(recreation_area.recreation_area_id) == str(recreation_area_id):
                return hostname
        return None

    def _api_request(
        self,
        rec_area_id: int,
        endpoint_name: str,
        params: Optional[Dict[str, str]] = None,
    ) -> Any:
        if params is None:
            params = {}

        hostname = self._hostname_for(rec_area_id)
        endpoint = ENDPOINTS.get(endpoint_name)
        url = None
        if endpoint:
            url = endpoint.format(hostname)
        if not url:
            raise CamplyError(f"Endpoint not found: {endpoint_name}")
        headers = {
            "User-Agent": UserAgent(browsers=["chrome"]).random,
            "Accept-Language": "en-US,en;q=0.9",
        }
        response = self.session.get(
            url=str(url), headers=headers, params=params, timeout=30
        )
        if response.ok is False:
            error_message = f"Receiving bad data from GoingToCamp API: status_code: {response.status_code}: {response.text}"
            logger.error(error_message)
            raise ConnectionError(error_message)

        return json.loads(response.content)

    def _filter_facilities_responses(
        self, rec_area_id: int, facilities: Any = List[Dict[str, Any]]
    ) -> List[ResourceLocation]:
        """
        Filter Facilities to Actual Reservable Campsites

        Parameters
        ----------
        rec_area_id: int
            Recreation Area ID
        facilities: List[Dict[str, Any]]
            List of facilities

        Returns
        -------
        List[ResourceLocation]
        """
        filtered_facilities = []
        for facil in facilities:
            try:
                location_name = str(
                    _fetch_nested_key(
                        facil,
                        "localizedValues",
                        0,
                        "fullName",
                    )
                )
                park_alerts = _fetch_nested_key(
                    facil,
                    "park_alerts",
                    "en-US",
                    0,
                    "messageTitle",
                )
                if not park_alerts:
                    park_alerts = _fetch_nested_key(
                        facil,
                        "park_alerts",
                        "en-CA",
                        0,
                        "messageTitle",
                    )

                region_name = _fetch_nested_key(facil, "region")

                facility = ResourceLocation(
                    id=None,
                    region_name=region_name if region_name else "",
                    park_alerts=park_alerts,
                    rec_area_id=rec_area_id,
                    resource_categories=facil.get("resourceCategoryIds"),
                    resource_location_id=facil.get("resourceLocationId"),
                    resource_location_name=location_name,
                )
            except ValidationError as ve:
                logger.error("That doesn't look like a valid Campground Facility")
                logger.error(facil)
                raise ProviderSearchError(
                    "Invalid Campground Facility Returned"
                ) from ve

            if not facility.resource_categories:
                continue

            # Resource categories from: /api/resourcecategory
            if any(
                [
                    CAMP_SITE in facility.resource_categories,
                    GROUP_SITE in facility.resource_categories,
                    OVERFLOW_SITE in facility.resource_categories,
                ]
            ):
                filtered_facilities.append(facility)

        return filtered_facilities

    def _process_facilities_responses(
        self, rec_area: RecreationArea, facility: ResourceLocation
    ) -> Tuple[ResourceLocation, CampgroundFacility]:
        """
        Process the facilities responses

        Parameters
        ----------
        facility: dict

        Returns
        -------
        Tuple[ResourceLocation, CampgroundFacility]
        """
        facility.id = _fetch_nested_key(
            self.campground_details,
            facility.resource_location_id,
            "mapId",
        )
        if facility.region_name:
            formatted_recreation_area = (
                f"{rec_area.recreation_area}, {facility.region_name}"
            )
        else:
            formatted_recreation_area = f"{rec_area.recreation_area}"

        campground_facility = CampgroundFacility(
            facility_name=facility.resource_location_name,
            recreation_area=formatted_recreation_area,
            facility_id=int(facility.resource_location_id)
            if facility.resource_location_id
            else 0,
            recreation_area_id=facility.rec_area_id,
            map_id=facility.id,
        )
        return facility, campground_facility

    def _find_matching_resources(
        self, rec_area_id: int, search_filter: Dict[str, Any]
    ) -> Any:
        results = self._api_request(rec_area_id, "MAPDATA", search_filter)
        availability_details = {
            search_filter["mapId"]: results["resourceAvailabilities"]
        }

        return availability_details, list(results["mapLinkAvailabilities"].keys())

    def list_equipment_types(self, rec_area_id: int) -> List[GoingToCampEquipment]:
        """
        List equipment types available for a recreation area

        Params
        ------
        rec_area_id: int
            The ID of the recreation area

        Returns
        -------
        types: List[GoingToCampEquipment]
            A list of equipment types available to this rec area
        """
        results = self._api_request(rec_area_id, "LIST_EQUIPMENT")

        if not isinstance(results, list) or len(results) == 0:
            return []

        equipment_types: List[GoingToCampEquipment] = []
        # Only allow equipment from non-group equipment category (the 0th
        # element in results)
        for sub_category in results[0].get("subEquipmentCategories", []):
            equipment_name = str(
                _fetch_nested_key(
                    sub_category,
                    "localizedValues",
                    0,
                    "name",
                )
            )
            equipment_id = int(sub_category.get("subEquipmentCategoryId", 0))
            equipment_types.append(
                GoingToCampEquipment(
                    equipment_name=equipment_name,
                    equipment_type_id=equipment_id,
                )
            )

        log_sorted_response(response_array=equipment_types)
        return equipment_types

    def list_site_availability(
        self,
        campground: CampgroundFacility,
        start_date: datetime.date,
        end_date: datetime.date,
        equipment_type_id: Optional[str],
    ) -> List[AvailableResource]:
        """
        Retrieve the Availability for all Sites in a Camp Area

        Sites are filtered on the provided date range and compatible
        equipment.

        Returns
        -------
        available_sites: List[AvailableResource]
            The list of available sites
        """
        search_filter: Dict[str, Any] = {
            "mapId": campground.map_id,
            "resourceLocationId": campground.facility_id,
            "bookingCategoryId": 0,
            "startDate": start_date.isoformat(),
            "endDate": end_date.isoformat(),
            "isReserving": True,
            "getDailyAvailability": False,
            "partySize": 1,
            "numEquipment": 1,
            "equipmentCategoryId": NON_GROUP_EQUIPMENT,
            "filterData": [],
        }
        if equipment_type_id:
            search_filter["subEquipmentCategoryId"] = equipment_type_id

        resources, additional_resources = self._find_matching_resources(
            int(campground.recreation_area_id),
            search_filter,
        )

        # Resources are often deeply nested; fetch nested resources
        for map_id in additional_resources:
            search_filter["mapId"] = map_id
            avail, _ = self._find_matching_resources(
                int(campground.recreation_area_id),
                search_filter,
            )
            resources.update(avail)

        availabilities = []
        for map_id, resource_details in resources.items():
            for resource_id, availability_details in resource_details.items():
                if availability_details[0]["availability"] == 0:
                    ar = AvailableResource(resource_id=resource_id, map_id=map_id)
                    availabilities.append(ar)

        return availabilities

    def find_campsites(
        self,
        *,
        search_months: List[datetime.datetime],
        campgrounds: List[CampgroundFacility],
        nights: int = 1,
        search_window: Optional[Union[List[Any], Any]] = None,
        equipment_id: Optional[int] = None,
        rec_area_id: Optional[int] = None,
        **kwargs: Any,
    ) -> List[AvailableCampsite]:
        """
        Search for all campsites matching search criteria.

        Because Going To Camp has no efficient way of filtering
        campsites for multiple campgrounds, this function is limited to listening
        all campsites _within_ a cammpground. Iterating through all sites for
        all campgrounds would likely lead to abuse complaints.

        Returns
        -------
        List[AvailableCampsite]
        """
        available_sites: List[AvailableCampsite] = []
        if not search_window or rec_area_id is None:
            return available_sites

        search_windows = make_list(search_window)
        if not search_windows:
            return available_sites

        for window in search_windows:
            current_start_date = window.get_current_start_date()
            for campground in campgrounds:
                sites = self.list_site_availability(
                    campground=campground,
                    start_date=current_start_date,
                    end_date=window.end_date,
                    equipment_type_id=str(equipment_id)
                    if equipment_id is not None
                    else None,
                )
                for site in sites:
                    site_details = self.get_site_details(
                        rec_area_id, int(site.resource_id)
                    )
                    booking_nights = (window.end_date - current_start_date).days
                    start_dt = datetime.datetime.combine(
                        current_start_date, datetime.time.min
                    )
                    end_dt = datetime.datetime.combine(
                        window.end_date, datetime.time.min
                    )
                    (
                        rec_area_domain_name,
                        rec_area,
                    ) = self.rec_area_lookup(rec_area_id=rec_area_id)
                    reservation_url = self.get_reservation_link(
                        rec_area_domain_name,
                        resource_location_id=campground.facility_id,
                        map_id=site.map_id,
                        equipment_id=NON_GROUP_EQUIPMENT,
                        sub_equipment_id=equipment_id,
                        party_size=1,
                        start_date=current_start_date,
                        end_date=window.end_date,
                    )

                    # Some rec areas have zero-capacity sites, which should not
                    # be viable for camping. Skip all zero-capacity sites.
                    if (
                        not site_details["minCapacity"]
                        or not site_details["maxCapacity"]
                    ):
                        continue

                    available_sites.append(
                        AvailableCampsite(
                            campsite_id=site_details["resourceId"],
                            campsite_site_name=site_details["localizedValues"][0][
                                "name"
                            ],
                            booking_date=start_dt,
                            booking_end_date=end_dt,
                            booking_nights=booking_nights,
                            campsite_loop_name="Unknown",
                            campsite_type=site_details["site_attributes"].get(
                                "Service Type", "Unknown"
                            ),
                            campsite_occupancy=(
                                site_details["minCapacity"],
                                site_details["maxCapacity"],
                            ),
                            campsite_use_type="N/A",
                            availability_status="Available",
                            recreation_area=rec_area.recreation_area,
                            recreation_area_id=rec_area_id,
                            facility_name=campground.facility_name,
                            facility_id=campground.facility_id,
                            booking_url=reservation_url,
                        )
                    )

        return available_sites


def _fetch_nested_key(obj: Union[dict[Any, Any], list[Any], object], *keys: Any) -> Any:
    """
    Fetch nested keys from dictionaries/lists if the keys exist

    Example
    -------
        mydict = {
            'foo': {
                'bar': 'baz'
            }
        }
        val = _fetch_nested_key(mydict, 'foo', 'bar')
        print(f"Value: {val}")
        Prints: Value: baz
    """
    if (
        not isinstance(obj, dict)
        and not isinstance(obj, list)
        and not isinstance(obj, object)
    ):
        raise AttributeError(
            "`obj` must be of type `dict`, `list`, or `object`, but is not"
        )
    if len(keys) == 0:
        raise AttributeError(
            "At least one key must be specified in `keys:`. None were provided"
        )

    _element: Any = obj
    for key in keys:
        try:
            if (
                isinstance(_element, (dict, list)) and key in _element
                if isinstance(_element, dict)
                else True
            ):
                _element = _element[key]
            else:
                _element = getattr(_element, key)
        except (KeyError, TypeError, AttributeError, IndexError):
            return None

    return _element
