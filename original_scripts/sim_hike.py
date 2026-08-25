import argparse
import datetime
import json
import math
import random
import re
import urllib.request
import urllib.error

from fit_tool.fit_file_builder import FitFileBuilder
from fit_tool.profile.messages.file_id_message import FileIdMessage
from fit_tool.profile.messages.device_info_message import DeviceInfoMessage
from fit_tool.profile.messages.sport_message import SportMessage
from fit_tool.profile.messages.event_message import EventMessage
from fit_tool.profile.messages.record_message import RecordMessage
from fit_tool.profile.messages.lap_message import LapMessage
from fit_tool.profile.messages.session_message import SessionMessage
from fit_tool.profile.messages.activity_message import ActivityMessage
from fit_tool.profile.profile_type import (
    FileType, Manufacturer, Event, EventType, Sport, SubSport, Activity,
    LapTrigger, SessionTrigger
)

def haversine(lon1, lat1, lon2, lat2):
    R = 6371e3  # Radius of earth in meters
    phi1, phi2 = math.radians(lat1), math.radians(lat2)
    dphi = math.radians(lat2 - lat1)
    dlambda = math.radians(lon2 - lon1)
    a = math.sin(dphi/2)**2 + math.cos(phi1)*math.cos(phi2)*math.sin(dlambda/2)**2
    return 2 * R * math.atan2(math.sqrt(a), math.sqrt(1-a))

def parse_kml_coordinates(kml_file) -> list[tuple[float, float, float]]:
    """Parse KML file and return list of (lon, lat, elevation) tuples."""
    with open(kml_file, 'r', encoding='utf-8') as f:
        content = f.read()

    matches = re.findall(r'<coordinates>(.*?)</coordinates>', content, re.DOTALL)
    if not matches:
        raise ValueError("Could not find any <coordinates> inside the KML file.")

    points = []
    for match in matches:
        chunks = match.strip().split()
        for chunk in chunks:
            parts = chunk.split(',')
            if len(parts) >= 2:
                lon = float(parts[0])
                lat = float(parts[1])
                ele = float(parts[2]) if len(parts) >= 3 else 0.0
                points.append((lon, lat, ele))

    return points

def fetch_elevation(points, batch_size=100):
    """Fetch elevation data from the Open-Elevation API for a list of (lon, lat, ele) points.
    Returns a new list with elevation data filled in. Processes in batches to avoid
    request size limits.
    """
    url = "https://api.open-elevation.com/api/v1/lookup"
    enriched = list(points)  # copy
    total = len(points)

    for start in range(0, total, batch_size):
        end = min(start + batch_size, total)
        batch = points[start:end]

        locations = [{"latitude": p[1], "longitude": p[0]} for p in batch]
        payload = json.dumps({"locations": locations}).encode("utf-8")

        req = urllib.request.Request(
            url,
            data=payload,
            headers={"Content-Type": "application/json", "Accept": "application/json"},
            method="POST"
        )

        try:
            with urllib.request.urlopen(req, timeout=30) as resp:
                data = json.loads(resp.read().decode("utf-8"))
                results = data.get("results", [])
                for i, result in enumerate(results):
                    ele = result.get("elevation", 0.0)
                    idx = start + i
                    enriched[idx] = (enriched[idx][0], enriched[idx][1], float(ele))
        except (urllib.error.URLError, urllib.error.HTTPError, TimeoutError, Exception) as e:
            print(f"  Warning: Failed to fetch elevation for batch {start}-{end}: {e}")
            # Leave elevation as 0 for this batch

        pct = min(100, round(end / total * 100))
        print(f"  Fetched elevation for {end}/{total} points ({pct}%)...")

    return enriched

def main():
    parser = argparse.ArgumentParser(
        description="Simulate a hiking FIT file using a KML route with elevation data. "
                    "If the KML has no altitude, elevation is fetched automatically from Open-Elevation API.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""Examples:
  Hike at 3.5 km/h:
    python sim_hike.py --datetime "06-04-26 09:00:00" --speed 3.5 --kml trail.kml --file hike.fit

  Faster pace:
    python sim_hike.py --datetime "06-04-26 09:00:00" --speed 4.5 --kml trail.kml --file hike.fit
"""
    )
    parser.add_argument("--datetime", type=str, required=True, help="Start datetime in format 'dd-mm-yy HH:MM:SS'")
    parser.add_argument("--speed", type=float, required=True, help="Average hiking speed in km/h (e.g. 3.0 - 5.0)")
    parser.add_argument("--kml", type=str, required=True, help="Input KML file containing the route with elevation")
    parser.add_argument("--file", type=str, required=True, help="Output FIT filename")

    args = parser.parse_args()

    try:
        start_time = datetime.datetime.strptime(args.datetime, "%d-%m-%y %H:%M:%S")
    except ValueError:
        print(f"Error parsing datetime. Expected 'dd-mm-yy HH:MM:SS'. Got '{args.datetime}'")
        return

    points = parse_kml_coordinates(args.kml)
    if len(points) < 2:
        print("Not enough points found in the KML file to define a route.")
        return

    # Check if elevation data is present; fetch from API if not
    has_elevation = any(p[2] != 0.0 for p in points)
    if not has_elevation:
        print("No elevation data in KML. Fetching from Open-Elevation API...")
        points = fetch_elevation(points)
        has_elevation = any(p[2] != 0.0 for p in points)
        if not has_elevation:
            print("Warning: Could not fetch elevation data. Altitude fields will be 0.")

    # Calculate cumulative 2D distances (horizontal)
    dists = [0.0]
    for i in range(1, len(points)):
        d = haversine(points[i-1][0], points[i-1][1], points[i][0], points[i][1])
        dists.append(dists[-1] + d)

    total_dist_m = dists[-1]

    # Calculate total ascent/descent from raw KML elevations
    total_ascent_kml = 0.0
    total_descent_kml = 0.0
    for i in range(1, len(points)):
        ele_diff = points[i][2] - points[i-1][2]
        if ele_diff > 0:
            total_ascent_kml += ele_diff
        else:
            total_descent_kml += abs(ele_diff)

    # Duration from speed
    speed_m_s = args.speed * 1000.0 / 3600.0
    total_time_s = int(total_dist_m / speed_m_s)

    if total_time_s <= 0:
        print("Total route distance or speed is too small, duration=0.")
        return

    # Elevation range
    elevations = [p[2] for p in points]
    min_elevation = min(elevations)
    max_elevation = max(elevations)

    print(f"Route parsed. Total distance: {total_dist_m:.1f} meters ({total_dist_m/1000:.2f} km).")
    print(f"Average speed: {args.speed} km/h")
    print(f"Elevation: min {min_elevation:.0f}m / max {max_elevation:.0f}m")
    print(f"Total ascent: {total_ascent_kml:.0f}m  |  Total descent: {total_descent_kml:.0f}m")
    print(f"Calculated duration: {total_time_s} seconds ({total_time_s/60:.1f} mins).")

    start_timestamp_millis = round(start_time.timestamp()) * 1000
    current_timestamp = start_timestamp_millis

    # Compute bounding box from ALL route points
    all_lats = [p[1] for p in points]
    all_lons = [p[0] for p in points]
    bbox_min_lat = min(all_lats)
    bbox_max_lat = max(all_lats)
    bbox_min_lon = min(all_lons)
    bbox_max_lon = max(all_lons)

    # First coordinate for start position
    start_lat, start_lon = points[0][1], points[0][0]

    builder = FitFileBuilder(auto_define=True, min_string_size=50)

    # 1. FileIdMessage
    file_id = FileIdMessage()
    file_id.type = FileType.ACTIVITY
    file_id.manufacturer = Manufacturer.GARMIN.value
    file_id.product = 4400
    file_id.time_created = start_timestamp_millis
    file_id.serial_number = 345000124
    builder.add(file_id)

    # 2. DeviceInfoMessage
    device_info = DeviceInfoMessage()
    device_info.timestamp = start_timestamp_millis
    device_info.manufacturer = Manufacturer.GARMIN.value
    device_info.product = 4400
    device_info.serial_number = 345000124
    device_info.software_version = 14.50
    device_info.device_index = 0
    builder.add(device_info)

    # 3. Sport Message
    sport_msg = SportMessage()
    sport_msg.sport = Sport.HIKING
    sport_msg.sub_sport = SubSport.GENERIC
    sport_msg.sport_name = "Hiking"
    builder.add(sport_msg)

    # 4. Start Timer Event
    start_event = EventMessage()
    start_event.event = Event.TIMER
    start_event.event_type = EventType.START
    start_event.timestamp = start_timestamp_millis
    builder.add(start_event)

    # 5. Generate Records (1 record per second)
    #
    # Realistic HR model for hiking:
    #   - Resting HR ~68-78 bpm
    #   - Flat terrain: 100-120 bpm
    #   - Uphill: rises to 130-160 bpm depending on grade
    #   - Downhill: drops to 95-115 bpm
    #   - Hard cap at 175 bpm
    #
    RESTING_HR = random.uniform(68, 78)
    MAX_HR_CAP = 175
    hr = RESTING_HR
    records = []
    all_hr_values = []

    # Hiking cadence: ~95-115 steps per minute (slower than walking)
    cadence_base = 105.0
    total_steps = 0

    # Track last recorded position for lap end_position
    last_lat, last_lon = start_lat, start_lon

    # Track elevation stats from interpolated points
    total_ascent = 0.0
    total_descent = 0.0
    prev_altitude = points[0][2]
    all_altitudes = []

    # Calories: ~350-500 kcal/hr for hiking depending on terrain
    cal_per_hour = 350 + args.speed * 30
    total_calories = round(total_time_s * (cal_per_hour / 3600))

    # Warm-up phase: first 120 seconds
    warmup_duration = min(120, total_time_s // 4)

    pt_idx = 0

    for t in range(total_time_s + 1):
        target_dist = t * speed_m_s

        # Advance point index
        while pt_idx < len(points) - 2 and dists[pt_idx + 1] < target_dist:
            pt_idx += 1

        # Interpolate position and elevation
        d_start = dists[pt_idx]
        d_end = dists[pt_idx + 1]

        fraction = 0.0
        if d_end > d_start:
            fraction = (target_dist - d_start) / (d_end - d_start)
            fraction = max(0.0, min(1.0, fraction))

        lon = points[pt_idx][0] + fraction * (points[pt_idx+1][0] - points[pt_idx][0])
        lat = points[pt_idx][1] + fraction * (points[pt_idx+1][1] - points[pt_idx][1])
        altitude = points[pt_idx][2] + fraction * (points[pt_idx+1][2] - points[pt_idx][2])

        last_lat, last_lon = lat, lon

        # Calculate grade (slope) between this point and previous
        ele_diff = altitude - prev_altitude
        if ele_diff > 0:
            total_ascent += ele_diff
        else:
            total_descent += abs(ele_diff)

        # Grade as percentage (rise/run * 100), using 1-second distance
        segment_dist = speed_m_s  # approximate horizontal distance in 1 second
        grade_pct = (ele_diff / segment_dist * 100) if segment_dist > 0 else 0.0
        grade_pct = max(-50, min(50, grade_pct))  # clamp to reasonable range

        all_altitudes.append(altitude)
        prev_altitude = altitude

        # Speed variation: slightly slower uphill, faster downhill
        grade_speed_factor = 1.0 - grade_pct * 0.005  # 10% grade -> 5% slower
        grade_speed_factor = max(0.7, min(1.2, grade_speed_factor))
        instant_speed = speed_m_s * grade_speed_factor * random.uniform(0.96, 1.04)

        record = RecordMessage()
        record.timestamp = current_timestamp
        record.position_lat = lat
        record.position_long = lon
        record.distance = target_dist
        record.speed = instant_speed
        record.altitude = altitude
        record.enhanced_altitude = altitude
        record.grade = grade_pct

        # HR simulation based on grade
        if t < warmup_duration:
            # Warm-up ramp
            warmup_progress = t / warmup_duration
            base_target = RESTING_HR + (110 - RESTING_HR) * (1 - (1 - warmup_progress) ** 2)
        else:
            # Base target ~110 bpm on flat terrain
            base_target = 110.0

        # Grade influence on HR: uphill raises HR, downhill lowers it
        grade_hr_offset = grade_pct * 3.0  # +10% grade -> +30 bpm
        target_hr = base_target + grade_hr_offset
        target_hr = max(RESTING_HR, min(MAX_HR_CAP - 5, target_hr))

        # Smooth convergence + jitter
        hr += (target_hr - hr) * random.uniform(0.04, 0.12)
        hr += random.uniform(-1.2, 1.2)
        hr = max(RESTING_HR - 3, min(MAX_HR_CAP, hr))
        record.heart_rate = round(hr)
        all_hr_values.append(round(hr))

        # Cadence: slower uphill, slightly faster downhill
        cadence_offset = -grade_pct * 1.5  # uphill -> slower steps
        cadence = cadence_base + cadence_offset + random.uniform(-4, 4)
        cadence = max(80, min(125, cadence))
        record.cadence = round(cadence)
        total_steps += cadence / 60.0

        records.append(record)
        current_timestamp += 1000

    builder.add_all(records)

    # 6. Stop Timer Event
    stop_event = EventMessage()
    stop_event.event = Event.TIMER
    stop_event.event_type = EventType.STOP
    stop_event.timestamp = current_timestamp
    builder.add(stop_event)

    total_strides = round(total_steps / 2)

    # Compute HR stats
    avg_hr = round(sum(all_hr_values) / len(all_hr_values)) if all_hr_values else 0
    max_hr = max(all_hr_values) if all_hr_values else 0
    min_hr = min(all_hr_values) if all_hr_values else 0

    # Compute altitude stats
    avg_alt = sum(all_altitudes) / len(all_altitudes) if all_altitudes else 0
    min_alt = min(all_altitudes) if all_altitudes else 0
    max_alt = max(all_altitudes) if all_altitudes else 0

    # 7. Lap Message
    lap = LapMessage()
    lap.timestamp = current_timestamp
    lap.start_time = start_timestamp_millis
    lap.total_elapsed_time = total_time_s
    lap.total_timer_time = total_time_s
    lap.total_distance = total_dist_m
    lap.total_calories = total_calories
    lap.total_strides = total_strides
    lap.avg_heart_rate = avg_hr
    lap.max_heart_rate = max_hr
    lap.total_ascent = round(total_ascent)
    lap.total_descent = round(total_descent)
    lap.avg_altitude = avg_alt
    lap.min_altitude = min_alt
    lap.max_altitude = max_alt
    lap.enhanced_avg_altitude = avg_alt
    lap.enhanced_min_altitude = min_alt
    lap.enhanced_max_altitude = max_alt
    lap.start_position_lat = start_lat
    lap.start_position_long = start_lon
    lap.end_position_lat = last_lat
    lap.end_position_long = last_lon
    lap.sport = Sport.HIKING
    lap.sub_sport = SubSport.GENERIC
    lap.lap_trigger = LapTrigger.SESSION_END
    builder.add(lap)

    # 8. Session Message
    session = SessionMessage()
    session.timestamp = current_timestamp
    session.start_time = start_timestamp_millis
    session.total_elapsed_time = total_time_s
    session.total_timer_time = total_time_s
    session.sport = Sport.HIKING
    session.sub_sport = SubSport.GENERIC
    session.first_lap_index = 0
    session.num_laps = 1
    session.total_distance = total_dist_m
    session.total_calories = total_calories
    session.total_strides = total_strides
    session.avg_heart_rate = avg_hr
    session.max_heart_rate = max_hr
    session.total_ascent = round(total_ascent)
    session.total_descent = round(total_descent)
    session.avg_altitude = avg_alt
    session.min_altitude = min_alt
    session.max_altitude = max_alt
    session.enhanced_avg_altitude = avg_alt
    session.enhanced_min_altitude = min_alt
    session.enhanced_max_altitude = max_alt
    session.start_position_lat = start_lat
    session.start_position_long = start_lon
    session.nec_lat = bbox_max_lat
    session.nec_long = bbox_max_lon
    session.swc_lat = bbox_min_lat
    session.swc_long = bbox_min_lon
    session.trigger = SessionTrigger.ACTIVITY_END
    builder.add(session)

    # 9. Activity Message
    activity = ActivityMessage()
    activity.timestamp = current_timestamp
    activity.total_timer_time = total_time_s
    activity.num_sessions = 1
    activity.type = Activity.MANUAL
    activity.event = Event.ACTIVITY
    activity.event_type = EventType.STOP
    builder.add(activity)

    fit_file = builder.build()
    fit_file.to_file(args.file)

    hours = total_time_s // 3600
    minutes = (total_time_s % 3600) // 60
    seconds = total_time_s % 60
    print(f"Total steps: {round(total_steps)}")
    print(f"Total strides: {total_strides}")
    print(f"Calories: ~{total_calories} kcal")
    print(f"Heart Rate: avg {avg_hr} / max {max_hr} / min {min_hr} bpm")
    print(f"Elevation: avg {avg_alt:.0f}m / min {min_alt:.0f}m / max {max_alt:.0f}m")
    print(f"Ascent: {total_ascent:.0f}m  |  Descent: {total_descent:.0f}m")
    print(f"Duration: {hours:02d}:{minutes:02d}:{seconds:02d}")
    print(f"Successfully generated {args.file} as a valid FIT hiking activity file.")

if __name__ == "__main__":
    main()
