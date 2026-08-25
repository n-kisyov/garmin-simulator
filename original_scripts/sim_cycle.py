import argparse
import datetime
import math
import random
import re

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
    FileType,
    Manufacturer,
    Event,
    EventType,
    Sport,
    SubSport,
    Activity,
    LapTrigger,
    SessionTrigger,
)


def haversine(lon1, lat1, lon2, lat2):
    R = 6371e3  # Radius of earth in meters
    phi1, phi2 = math.radians(lat1), math.radians(lat2)
    dphi = math.radians(lat2 - lat1)
    dlambda = math.radians(lon2 - lon1)

    a = math.sin(dphi / 2) ** 2 + math.cos(phi1) * math.cos(phi2) * math.sin(dlambda / 2) ** 2
    return 2 * R * math.atan2(math.sqrt(a), math.sqrt(1 - a))


def parse_kml_coordinates(kml_file) -> list[tuple[float, float]]:
    with open(kml_file, 'r', encoding='utf-8') as f:
        content = f.read()

    # Find the <coordinates> blocks. Ignore namespaces.
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
                points.append((lon, lat))

    return points


def main():
    parser = argparse.ArgumentParser(
        description="Simulate a cycling FIT file using a KML route."
    )
    parser.add_argument(
        "--datetime",
        type=str,
        required=True,
        help="Start datetime in format 'dd-mm-yy HH:MM:SS'",
    )
    parser.add_argument(
        "--speed",
        type=float,
        required=True,
        help="Average cycling speed in km/h (e.g. 18.0 - 35.0)",
    )
    parser.add_argument(
        "--kml",
        type=str,
        required=True,
        help="Input KML file containing the route",
    )
    parser.add_argument(
        "--file",
        type=str,
        required=True,
        help="Output FIT filename",
    )

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

    # Calculate cumulative distances along the KML polyline
    dists = [0.0]
    for i in range(1, len(points)):
        d = haversine(points[i - 1][0], points[i - 1][1], points[i][0], points[i][1])
        dists.append(dists[-1] + d)

    total_dist_m = dists[-1]

    # Calculate duration based on average speed
    speed_m_s = args.speed * 1000.0 / 3600.0
    total_time_s = int(total_dist_m / speed_m_s)

    if total_time_s <= 0:
        print("Total route distance or speed is too small, duration=0.")
        return

    # Pace in min/km (useful check)
    pace_min_per_km = (total_time_s / 60.0) / (total_dist_m / 1000.0)

    print(f"Route parsed. Total distance: {total_dist_m:.1f} meters ({total_dist_m/1000:.2f} km).")
    print(
        f"Average speed: {args.speed:.1f} km/h | "
        f"Pace: {int(pace_min_per_km)}:{int((pace_min_per_km % 1) * 60):02d} min/km"
    )
    print(f"Calculated duration: {total_time_s} seconds ({total_time_s/60:.2f} mins).")

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

    # 3. Sport Message (Cycling)
    sport_msg = SportMessage()
    sport_msg.sport = Sport.CYCLING
    sport_msg.sub_sport = SubSport.GENERIC
    sport_msg.sport_name = "Cycling"
    builder.add(sport_msg)

    # 4. Start Timer Event
    start_event = EventMessage()
    start_event.event = Event.TIMER
    start_event.event_type = EventType.START
    start_event.timestamp = start_timestamp_millis
    builder.add(start_event)

    # 5. Generate Records (1 record per second)

    # Realistic HR model for steady cycling:
    # - Starts around 60-75 bpm, ramps up first few minutes
    # - Settles into 130-165 bpm depending on effort
    # - Natural variation ±2-3 bpm
    # - Hard cap at ~180 bpm
    RESTING_HR = random.uniform(60, 72)
    MAX_HR_CAP = 180
    hr = RESTING_HR
    records = []
    all_hr_values = []

    # Cycling cadence: ~80–95 rpm typical for steady riding
    cadence_base = 85.0
    total_revs = 0.0

    # Track last recorded position for lap end_position
    last_lat, last_lon = start_lat, start_lon

    # Steady-state HR target based on speed (simple linear mapping)
    # ~20 km/h → ~135 bpm, 25 km/h → ~145 bpm, 30 km/h → ~155 bpm, 35 km/h → ~165 bpm
    steady_hr = min(95 + args.speed * 2.0, MAX_HR_CAP - 5)

    # Calories: rough model ~500–900 kcal/hr depending on speed
    cal_per_hour = 450 + args.speed * 15
    total_calories = round(total_time_s * (cal_per_hour / 3600))

    # Warm-up phase: first 5 minutes or up to 1/3 of activity
    warmup_duration = min(300, total_time_s // 3)

    pt_idx = 0

    for t in range(total_time_s + 1):
        target_dist = t * speed_m_s

        # Advance point index to match target distance
        while pt_idx < len(points) - 2 and dists[pt_idx + 1] < target_dist:
            pt_idx += 1

        # Interpolate position along the segment
        d_start = dists[pt_idx]
        d_end = dists[pt_idx + 1]

        fraction = 0.0
        if d_end > d_start:
            fraction = (target_dist - d_start) / (d_end - d_start)
            fraction = max(0.0, min(1.0, fraction))

        lon = points[pt_idx][0] + fraction * (points[pt_idx + 1][0] - points[pt_idx][0])
        lat = points[pt_idx][1] + fraction * (points[pt_idx + 1][1] - points[pt_idx][1])

        last_lat, last_lon = lat, lon

        # Speed variation: ±10% natural fluctuation
        instant_speed = speed_m_s * random.uniform(0.9, 1.1)

        record = RecordMessage()
        record.timestamp = current_timestamp
        record.position_lat = lat
        record.position_long = lon
        record.distance = target_dist
        record.speed = instant_speed

        # HR simulation
        if t < warmup_duration:
            # Warm-up ramp from resting toward steady state
            warmup_progress = t / warmup_duration
            target_hr = RESTING_HR + (steady_hr - RESTING_HR) * (1 - (1 - warmup_progress) ** 2)
        else:
            # Steady state with small upward drift over long rides
            elapsed_after_warmup = t - warmup_duration
            drift = min(elapsed_after_warmup * 0.002, 6)  # up to +6 bpm
            target_hr = steady_hr + drift

        # Smooth convergence + jitter
        hr += (target_hr - hr) * random.uniform(0.05, 0.15)
        hr += random.uniform(-1.5, 1.5)
        hr = max(RESTING_HR, min(MAX_HR_CAP, hr))

        record.heart_rate = round(hr)
        all_hr_values.append(round(hr))

        # Cadence with natural variation (rpm)
        cadence = cadence_base + random.uniform(-7, 7)
        cadence = max(70, min(100, cadence))
        record.cadence = round(cadence)

        # Accumulate total crank revolutions
        total_revs += cadence / 60.0  # rpm → revs per second

        records.append(record)
        current_timestamp += 1000

    builder.add_all(records)

    # 6. Stop Timer Event
    stop_event = EventMessage()
    stop_event.event = Event.TIMER
    stop_event.event_type = EventType.STOP
    stop_event.timestamp = current_timestamp
    builder.add(stop_event)

    # HR stats
    avg_hr = round(sum(all_hr_values) / len(all_hr_values)) if all_hr_values else 0
    max_hr = max(all_hr_values) if all_hr_values else 0
    min_hr = min(all_hr_values) if all_hr_values else 0

    total_revs_int = round(total_revs)

    # 7. Lap Message — use actual last recorded GPS point as end position
    lap = LapMessage()
    lap.timestamp = current_timestamp
    lap.start_time = start_timestamp_millis
    lap.total_elapsed_time = total_time_s
    lap.total_timer_time = total_time_s
    lap.total_distance = total_dist_m
    lap.total_calories = total_calories
    # Use total_strides field as a generic counter (mirroring run script)
    lap.total_strides = total_revs_int
    lap.avg_heart_rate = avg_hr
    lap.max_heart_rate = max_hr
    lap.start_position_lat = start_lat
    lap.start_position_long = start_lon
    lap.end_position_lat = last_lat
    lap.end_position_long = last_lon
    lap.sport = Sport.CYCLING
    lap.sub_sport = SubSport.GENERIC
    lap.lap_trigger = LapTrigger.SESSION_END
    builder.add(lap)

    # 8. Session Message — bounding box from ALL route points
    session = SessionMessage()
    session.timestamp = current_timestamp
    session.start_time = start_timestamp_millis
    session.total_elapsed_time = total_time_s
    session.total_timer_time = total_time_s
    session.sport = Sport.CYCLING
    session.sub_sport = SubSport.GENERIC
    session.first_lap_index = 0
    session.num_laps = 1
    session.total_distance = total_dist_m
    session.total_calories = total_calories
    session.total_strides = total_revs_int
    session.avg_heart_rate = avg_hr
    session.max_heart_rate = max_hr
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

    print(f"Total crank revolutions: {total_revs_int}")
    print(f"Calories: ~{total_calories} kcal")
    print(f"Heart Rate: avg {avg_hr} / max {max_hr} / min {min_hr} bpm")
    print(f"Duration: {hours:02d}:{minutes:02d}:{seconds:02d}")
    print(f"Successfully generated {args.file} as a valid FIT cycling activity file.")


if __name__ == "__main__":
    main()